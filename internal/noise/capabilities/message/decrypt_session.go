package message

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

	"wa-api/internal/noise/capabilities/send"
	"wa-api/internal/noise/persistence/store"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/msgpad"
	"wa-api/internal/noise/protocol/types"
)

// BufferedDecrypt roda `decrypt` com desduplicacao por hash do ciphertext,
// quando o buffer de eventos decifrados esta' ligado.
//
// A montagem do hash e' FORMATO PERSISTIDO: ciphertext, depois cada parte extra
// precedida de um byte zero, e dois bytes zero no fim. Mudar qualquer detalhe
// disso invalida em silencio todo o buffer ja' gravado. Ver o doc dos
// separadores em constants.go.
func BufferedDecrypt(
	ctx context.Context,
	t Transport,
	ciphertext []byte,
	serverTimestamp time.Time,
	decrypt func(context.Context) ([]byte, error),
	extraHashData ...string,
) (plaintext []byte, ciphertextHash [32]byte, err error) {
	if !t.EnableDecryptedEventBuffer() {
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
	buf, err = t.Store().EventBuffer.GetBufferedEvent(ctx, ciphertextHash)
	if err != nil {
		err = fmt.Errorf("failed to get buffered event: %w", err)
		return
	} else if buf != nil {
		if buf.Plaintext == nil {
			zerolog.Ctx(ctx).Debug().
				Hex("ciphertext_hash", ciphertextHash[:]).
				Time("insertion_time", buf.InsertTime).
				Msg("Returning event already processed error")
			err = fmt.Errorf("%w at %s", ErrEventAlreadyProcessed, buf.InsertTime.String())
			return
		}
		zerolog.Ctx(ctx).Debug().
			Hex("ciphertext_hash", ciphertextHash[:]).
			Time("insertion_time", buf.InsertTime).
			Msg("Returning previously decrypted plaintext")
		plaintext = buf.Plaintext
		return
	}

	err = t.Store().EventBuffer.DoDecryptionTxn(ctx, func(ctx context.Context) (innerErr error) {
		plaintext, innerErr = decrypt(ctx)
		if innerErr != nil {
			return
		}
		innerErr = t.Store().EventBuffer.PutBufferedEvent(ctx, ciphertextHash, plaintext, serverTimestamp)
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

// DecryptDM decifra um <enc> de conversa direta (pkmsg ou msg).
//
// `store.SignalProtobufSerializer` e' usado DIRETO, e nao pelo `pbSerializer`
// da raiz: sao o mesmo valor (a raiz declara `var pbSerializer =
// store.SignalProtobufSerializer`) e `store` ja' e' folha. Mesmo racional do
// lote 8.
//
// O ramo de AutoTrustIdentity foi preservado literalmente, inclusive o fato de
// a retentativa acontecer DENTRO do fechamento passado a BufferedDecrypt — ou
// seja, dentro da transacao de decifragem — e de o plaintext retornado ser o da
// segunda tentativa.
func DecryptDM(ctx context.Context, t Transport, child *waBinary.Node, from types.JID, isPreKey bool, serverTS time.Time) ([]byte, *[32]byte, error) {
	content, ok := child.Content.([]byte)
	if !ok {
		return nil, nil, fmt.Errorf("message content is not a byte slice")
	}

	pbSerializer := store.SignalProtobufSerializer
	builder := session.NewBuilderFromSignal(t.Store(), from.SignalAddress(), pbSerializer)
	cipher := session.NewCipher(builder, from.SignalAddress())
	var plaintext []byte
	var ciphertextHash [32]byte
	if isPreKey {
		preKeyMsg, err := protocol.NewPreKeySignalMessageFromBytes(content, pbSerializer.PreKeySignalMessage, pbSerializer.SignalMessage)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse prekey message: %w", err)
		}
		plaintext, ciphertextHash, err = BufferedDecrypt(ctx, t, content, serverTS, func(decryptCtx context.Context) ([]byte, error) {
			pt, innerErr := cipher.DecryptMessage(decryptCtx, preKeyMsg)
			if t.AutoTrustIdentity() && errors.Is(innerErr, signalerror.ErrUntrustedIdentity) {
				t.Log().Warnf("Got %v error while trying to decrypt prekey message from %s, clearing stored identity and retrying", innerErr, from)
				if innerErr = ClearUntrustedIdentity(decryptCtx, t, from); innerErr != nil {
					innerErr = fmt.Errorf("failed to clear untrusted identity: %w", innerErr)
					return nil, innerErr
				}
				pt, innerErr = cipher.DecryptMessage(decryptCtx, preKeyMsg)
			}
			return pt, innerErr
		}, ciphertextHashDomainPreKey, from.String())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decrypt prekey message: %w", err)
		}
	} else {
		msg, err := protocol.NewSignalMessageFromBytes(content, pbSerializer.SignalMessage)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse normal message: %w", err)
		}
		plaintext, ciphertextHash, err = BufferedDecrypt(ctx, t, content, serverTS, func(decryptCtx context.Context) ([]byte, error) {
			return cipher.Decrypt(decryptCtx, msg)
		}, ciphertextHashDomainNormal, from.String())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decrypt normal message: %w", err)
		}
	}
	var err error
	plaintext, err = msgpad.Unpad(plaintext, child.AttrGetter().Int(send.EncAttrVersion))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unpad message: %w", err)
	}
	return plaintext, &ciphertextHash, nil
}

// DecryptGroupMsg decifra um <enc type="skmsg"> de grupo.
//
// Repare que o erro de Unpad aqui volta NU, enquanto em DecryptDM ele e'
// embrulhado com "failed to unpad message". A assimetria existia antes da
// extracao e foi mantida: ela muda a mensagem de erro que aparece no log e em
// `events.UndecryptableMessage`.
func DecryptGroupMsg(ctx context.Context, t Transport, child *waBinary.Node, from types.JID, chat types.JID, serverTS time.Time) ([]byte, *[32]byte, error) {
	content, ok := child.Content.([]byte)
	if !ok {
		return nil, nil, fmt.Errorf("message content is not a byte slice")
	}

	pbSerializer := store.SignalProtobufSerializer
	senderKeyName := protocol.NewSenderKeyName(chat.String(), from.SignalAddress())
	builder := groups.NewGroupSessionBuilder(t.Store(), pbSerializer)
	cipher := groups.NewGroupCipher(builder, senderKeyName, t.Store())
	msg, err := protocol.NewSenderKeyMessageFromBytes(content, pbSerializer.SenderKeyMessage)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse group message: %w", err)
	}
	plaintext, ciphertextHash, err := BufferedDecrypt(ctx, t, content, serverTS, func(decryptCtx context.Context) ([]byte, error) {
		return cipher.Decrypt(decryptCtx, msg)
	}, ciphertextHashDomainSenderKey, chat.String(), from.String())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decrypt group message: %w", err)
	}
	plaintext, err = msgpad.Unpad(plaintext, child.AttrGetter().Int(send.EncAttrVersion))
	if err != nil {
		return nil, nil, err
	}
	return plaintext, &ciphertextHash, nil
}
