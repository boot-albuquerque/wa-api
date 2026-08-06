// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/protocol"
	"go.mau.fi/libsignal/session"
	"go.mau.fi/libsignal/signalerror"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

func (cli *Client) bufferedDecrypt(
	ctx context.Context,
	ciphertext []byte,
	serverTimestamp time.Time,
	decrypt func(context.Context) ([]byte, error),
	extraHashData ...string,
) (plaintext []byte, ciphertextHash [32]byte, err error) {
	if !cli.EnableDecryptedEventBuffer {
		plaintext, err = decrypt(ctx)
		return
	}
	hasher := sha256.New()
	hasher.Write(ciphertext)
	for _, part := range extraHashData {
		hasher.Write([]byte{0})
		hasher.Write([]byte(part))
	}
	hasher.Write([]byte{0, 0})
	ciphertextHash = *(*[32]byte)(hasher.Sum(nil))
	var buf *store.BufferedEvent
	buf, err = cli.Store.EventBuffer.GetBufferedEvent(ctx, ciphertextHash)
	if err != nil {
		err = fmt.Errorf("failed to get buffered event: %w", err)
		return
	} else if buf != nil {
		if buf.Plaintext == nil {
			zerolog.Ctx(ctx).Debug().
				Hex("ciphertext_hash", ciphertextHash[:]).
				Time("insertion_time", buf.InsertTime).
				Msg("Returning event already processed error")
			err = fmt.Errorf("%w at %s", EventAlreadyProcessed, buf.InsertTime.String())
			return
		}
		zerolog.Ctx(ctx).Debug().
			Hex("ciphertext_hash", ciphertextHash[:]).
			Time("insertion_time", buf.InsertTime).
			Msg("Returning previously decrypted plaintext")
		plaintext = buf.Plaintext
		return
	}

	err = cli.Store.EventBuffer.DoDecryptionTxn(ctx, func(ctx context.Context) (innerErr error) {
		plaintext, innerErr = decrypt(ctx)
		if innerErr != nil {
			return
		}
		innerErr = cli.Store.EventBuffer.PutBufferedEvent(ctx, ciphertextHash, plaintext, serverTimestamp)
		if innerErr != nil {
			innerErr = fmt.Errorf("failed to save decrypted event to buffer: %w", innerErr)
		}
		return
	})
	if err == nil {
		zerolog.Ctx(ctx).Debug().
			Hex("ciphertext_hash", ciphertextHash[:]).
			Msg("Successfully decrypted and saved event")
	}
	return
}

func (cli *Client) decryptDM(ctx context.Context, child *waBinary.Node, from types.JID, isPreKey bool, serverTS time.Time) ([]byte, *[32]byte, error) {
	content, ok := child.Content.([]byte)
	if !ok {
		return nil, nil, fmt.Errorf("message content is not a byte slice")
	}

	builder := session.NewBuilderFromSignal(cli.Store, from.SignalAddress(), pbSerializer)
	cipher := session.NewCipher(builder, from.SignalAddress())
	var plaintext []byte
	var ciphertextHash [32]byte
	if isPreKey {
		preKeyMsg, err := protocol.NewPreKeySignalMessageFromBytes(content, pbSerializer.PreKeySignalMessage, pbSerializer.SignalMessage)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse prekey message: %w", err)
		}
		plaintext, ciphertextHash, err = cli.bufferedDecrypt(ctx, content, serverTS, func(decryptCtx context.Context) ([]byte, error) {
			pt, innerErr := cipher.DecryptMessage(decryptCtx, preKeyMsg)
			if cli.AutoTrustIdentity && errors.Is(innerErr, signalerror.ErrUntrustedIdentity) {
				cli.Log.Warnf("Got %v error while trying to decrypt prekey message from %s, clearing stored identity and retrying", innerErr, from)
				if innerErr = cli.clearUntrustedIdentity(decryptCtx, from); innerErr != nil {
					innerErr = fmt.Errorf("failed to clear untrusted identity: %w", innerErr)
					return nil, innerErr
				}
				pt, innerErr = cipher.DecryptMessage(decryptCtx, preKeyMsg)
			}
			return pt, innerErr
		}, "prekey", from.String())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decrypt prekey message: %w", err)
		}
	} else {
		msg, err := protocol.NewSignalMessageFromBytes(content, pbSerializer.SignalMessage)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse normal message: %w", err)
		}
		plaintext, ciphertextHash, err = cli.bufferedDecrypt(ctx, content, serverTS, func(decryptCtx context.Context) ([]byte, error) {
			return cipher.Decrypt(decryptCtx, msg)
		}, "normal", from.String())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decrypt normal message: %w", err)
		}
	}
	var err error
	plaintext, err = unpadMessage(plaintext, child.AttrGetter().Int("v"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unpad message: %w", err)
	}
	return plaintext, &ciphertextHash, nil
}

func (cli *Client) decryptGroupMsg(ctx context.Context, child *waBinary.Node, from types.JID, chat types.JID, serverTS time.Time) ([]byte, *[32]byte, error) {
	content, ok := child.Content.([]byte)
	if !ok {
		return nil, nil, fmt.Errorf("message content is not a byte slice")
	}

	senderKeyName := protocol.NewSenderKeyName(chat.String(), from.SignalAddress())
	builder := groups.NewGroupSessionBuilder(cli.Store, pbSerializer)
	cipher := groups.NewGroupCipher(builder, senderKeyName, cli.Store)
	msg, err := protocol.NewSenderKeyMessageFromBytes(content, pbSerializer.SenderKeyMessage)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse group message: %w", err)
	}
	plaintext, ciphertextHash, err := cli.bufferedDecrypt(ctx, content, serverTS, func(decryptCtx context.Context) ([]byte, error) {
		return cipher.Decrypt(decryptCtx, msg)
	}, "senderkey", chat.String(), from.String())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt group message: %w", err)
	}
	plaintext, err = unpadMessage(plaintext, child.AttrGetter().Int("v"))
	if err != nil {
		return nil, nil, err
	}
	return plaintext, &ciphertextHash, nil
}
