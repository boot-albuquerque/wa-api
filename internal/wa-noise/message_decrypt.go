// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/libsignal/signalerror"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

var pbSerializer = store.SignalProtobufSerializer

func (cli *Client) handleEncryptedMessage(ctx context.Context, node *waBinary.Node) {
	info, err := cli.parseMessageInfo(node)
	if err != nil {
		cli.Log.Warnf("Failed to parse message: %v", err)
	} else {
		if !info.SenderAlt.IsEmpty() {
			cli.StoreLIDPNMapping(ctx, info.SenderAlt, info.Sender)
		} else if !info.RecipientAlt.IsEmpty() {
			cli.StoreLIDPNMapping(ctx, info.RecipientAlt, info.Chat)
		}
		if info.VerifiedName != nil && len(info.VerifiedName.Details.GetVerifiedName()) > 0 {
			go cli.updateBusinessName(ctx, info.Sender, info.SenderAlt, info, info.VerifiedName.Details.GetVerifiedName())
		}
		if len(info.PushName) > 0 && info.PushName != "-" && (cli.MessengerConfig == nil || info.PushName != "username") {
			go cli.updatePushName(ctx, info.Sender, info.SenderAlt, info, info.PushName)
		}
		if info.Sender.Server == types.NewsletterServer {
			var cancelled bool
			defer cli.maybeDeferredAck(ctx, node)(&cancelled)
			cancelled = cli.handlePlaintextMessage(ctx, info, node)
		} else {
			cli.decryptMessages(ctx, info, node)
		}
	}
}

func (cli *Client) handlePlaintextMessage(ctx context.Context, info *types.MessageInfo, node *waBinary.Node) (handlerFailed bool) {
	// TODO edits have an additional <meta msg_edit_t="1696321271735" original_msg_t="1696321248"/> node
	plaintext, ok := node.GetOptionalChildByTag("plaintext")
	if !ok {
		// 3:
		return
	}
	plaintextBody, ok := plaintext.Content.([]byte)
	if !ok {
		cli.Log.Warnf("Plaintext message from %s doesn't have byte content", info.SourceString())
		return
	}

	var msg waE2E.Message
	err := proto.Unmarshal(plaintextBody, &msg)
	if err != nil {
		cli.Log.Warnf("Error unmarshaling plaintext message from %s: %v", info.SourceString(), err)
		return
	}
	cli.storeMessageSecret(ctx, info, &msg)
	evt := &events.Message{
		Info:       *info,
		RawMessage: &msg,
	}
	meta, ok := node.GetOptionalChildByTag("meta")
	if ok {
		evt.NewsletterMeta = &events.NewsletterMessageMeta{
			EditTS:     meta.AttrGetter().UnixMilli("msg_edit_t"),
			OriginalTS: meta.AttrGetter().UnixTime("original_msg_t"),
		}
	}
	return cli.dispatchEvent(evt.UnwrapRaw())
}

func (cli *Client) migrateSessionStore(ctx context.Context, pn, lid types.JID) {
	err := cli.Store.Sessions.MigratePNToLID(ctx, pn, lid)
	if err != nil {
		cli.Log.Errorf("Failed to migrate signal store from %s to %s: %v", pn, lid, err)
	}
}

func (cli *Client) decryptMessages(ctx context.Context, info *types.MessageInfo, node *waBinary.Node) {
	unavailableNode, ok := node.GetOptionalChildByTag("unavailable")
	if ok && len(node.GetChildrenByTag(encNodeTag)) == 0 {
		uType := events.UnavailableType(unavailableNode.AttrGetter().String("type"))
		cli.Log.Warnf("Unavailable message %s from %s (type: %q)", info.ID, info.SourceString(), uType)
		cli.backgroundIfAsyncAck(func() {
			cli.immediateRequestMessageFromPhone(ctx, info)
			cli.sendAck(ctx, node, 0)
		})
		cli.dispatchEvent(&events.UndecryptableMessage{Info: *info, IsUnavailable: true, UnavailableType: uType})
		return
	}

	children := node.GetChildren()
	cli.Log.Debugf("Decrypting message from %s", info.SourceString())
	containsDirectMsg := false
	senderEncryptionJID := info.Sender
	if info.Sender.Server == types.DefaultUserServer && !info.Sender.IsBot() {
		if info.SenderAlt.Server == types.HiddenUserServer {
			senderEncryptionJID = info.SenderAlt
			cli.migrateSessionStore(ctx, info.Sender, info.SenderAlt)
		} else if lid, err := cli.Store.LIDs.GetLIDForPN(ctx, info.Sender); err != nil {
			cli.Log.Errorf("Failed to get LID for %s: %v", info.Sender, err)
		} else if !lid.IsEmpty() {
			cli.migrateSessionStore(ctx, info.Sender, lid)
			senderEncryptionJID = lid
			info.SenderAlt = lid
		} else {
			cli.Log.Warnf("No LID found for %s", info.Sender)
		}
	}
	var recognizedStanza, protobufFailed bool
	for _, child := range children {
		if child.Tag != encNodeTag {
			continue
		}
		recognizedStanza = true
		ag := child.AttrGetter()
		encType, ok := ag.GetString(encAttrType, false)
		if !ok {
			continue
		}
		var decrypted []byte
		var ciphertextHash *[32]byte
		var err error
		if encType == encTypePreKeyMsg || encType == encTypeMsg {
			decrypted, ciphertextHash, err = cli.decryptDM(ctx, &child, senderEncryptionJID, encType == encTypePreKeyMsg, info.Timestamp)
			containsDirectMsg = true
		} else if info.IsGroup && encType == encTypeSenderKey {
			decrypted, ciphertextHash, err = cli.decryptGroupMsg(ctx, &child, senderEncryptionJID, info.Chat, info.Timestamp)
		} else if encType == encTypeMsgSecret && info.Sender.IsBot() {
			targetSenderJID := info.MsgMetaInfo.TargetSender
			if targetSenderJID.User == "" {
				if info.Sender.Server == types.BotServer {
					targetSenderJID = cli.getOwnLID()
				} else {
					targetSenderJID = cli.getOwnID()
				}
			}
			var decryptMessageID string
			if info.MsgBotInfo.EditType == types.EditTypeInner || info.MsgBotInfo.EditType == types.EditTypeLast {
				decryptMessageID = info.MsgBotInfo.EditTargetID
			} else {
				decryptMessageID = info.ID
			}
			var msMsg waE2E.MessageSecretMessage
			var messageSecret []byte
			if messageSecret, _, err = cli.Store.MsgSecrets.GetMessageSecret(ctx, info.Chat, targetSenderJID, info.MsgMetaInfo.TargetID); err != nil {
				err = fmt.Errorf("failed to get message secret for %s: %v", info.MsgMetaInfo.TargetID, err)
			} else if messageSecret == nil {
				err = fmt.Errorf("message secret for %s not found", info.MsgMetaInfo.TargetID)
			} else if err = proto.Unmarshal(child.Content.([]byte), &msMsg); err != nil {
				err = fmt.Errorf("failed to unmarshal MessageSecretMessage protobuf: %v", err)
			} else {
				decrypted, err = cli.decryptBotMessage(ctx, messageSecret, &msMsg, decryptMessageID, targetSenderJID, info)
			}
		} else {
			cli.Log.Warnf("Unhandled encrypted message (type %s) from %s", encType, info.SourceString())
			continue
		}

		if errors.Is(err, EventAlreadyProcessed) {
			cli.Log.Debugf("Ignoring message %s from %s: %v", info.ID, info.SourceString(), err)
			continue
		} else if errors.Is(err, signalerror.ErrOldCounter) {
			cli.Log.Warnf("Ignoring message %s from %s: %v", info.ID, info.SourceString(), err)
			continue
		} else if err != nil {
			cli.Log.Warnf("Error decrypting message %s from %s: %v", info.ID, info.SourceString(), err)
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return
			}
			isUnavailable := encType == encTypeSenderKey && !containsDirectMsg && errors.Is(err, signalerror.ErrNoSenderKeyForUser)
			if encType == encTypeMsgSecret {
				cli.backgroundIfAsyncAck(func() {
					cli.sendAck(ctx, node, NackMissingMessageSecret)
				})
			} else if cli.SynchronousAck {
				cli.sendRetryReceipt(ctx, node, info, isUnavailable)
				// TODO this probably isn't supposed to ack
				cli.sendAck(ctx, node, 0)
			} else {
				go cli.sendRetryReceipt(context.WithoutCancel(ctx), node, info, isUnavailable)
				go cli.sendAck(ctx, node, 0)
			}
			cli.dispatchEvent(&events.UndecryptableMessage{
				Info:            *info,
				IsUnavailable:   isUnavailable,
				DecryptFailMode: events.DecryptFailMode(ag.OptionalString(encAttrDecryptFail)),
			})
			return
		}
		retryCount := ag.OptionalInt("count")
		cli.cancelDelayedRequestFromPhone(info.ID)

		var msg waE2E.Message
		var handlerFailed bool
		switch ag.Int(encAttrVersion) {
		case 2:
			err = proto.Unmarshal(decrypted, &msg)
			if err != nil {
				cli.Log.Warnf("Error unmarshaling decrypted message from %s: %v", info.SourceString(), err)
				protobufFailed = true
				continue
			}
			protobufFailed = false
			handlerFailed = cli.handleDecryptedMessage(ctx, info, &msg, retryCount)
		case 3:
			handlerFailed, protobufFailed = cli.handleDecryptedArmadillo(ctx, info, decrypted, retryCount)
		default:
			cli.Log.Warnf("Unknown version %d in decrypted message from %s", ag.Int(encAttrVersion), info.SourceString())
		}
		if handlerFailed {
			cli.Log.Warnf("Handler for %s failed", info.ID)
			return
		}
		if ciphertextHash != nil && cli.EnableDecryptedEventBuffer {
			// Use the context passed to decryptMessages
			err = cli.Store.EventBuffer.ClearBufferedEventPlaintext(ctx, *ciphertextHash)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).
					Hex("ciphertext_hash", ciphertextHash[:]).
					Str("message_id", info.ID).
					Msg("Failed to clear buffered event plaintext")
			} else {
				zerolog.Ctx(ctx).Debug().
					Hex("ciphertext_hash", ciphertextHash[:]).
					Str("message_id", info.ID).
					Msg("Deleted event plaintext from buffer")
			}

			if time.Since(cli.lastDecryptedBufferClear) > decryptedBufferClearInterval && ctx.Err() == nil {
				cli.lastDecryptedBufferClear = time.Now()
				go func() {
					err := cli.Store.EventBuffer.DeleteOldBufferedHashes(context.WithoutCancel(ctx))
					if err != nil {
						zerolog.Ctx(ctx).Err(err).Msg("Failed to delete old buffered hashes")
					}
				}()
			}
		}
	}
	cli.backgroundIfAsyncAck(func() {
		if !recognizedStanza {
			cli.sendAck(ctx, node, NackUnrecognizedStanza)
		} else if protobufFailed {
			cli.sendAck(ctx, node, NackInvalidProtobuf)
		} else {
			cli.sendMessageReceipt(ctx, info, node)
		}
	})
	return
}

func (cli *Client) clearUntrustedIdentity(ctx context.Context, target types.JID) error {
	err := cli.Store.Identities.DeleteIdentity(ctx, target.SignalAddress().String())
	if err != nil {
		return fmt.Errorf("failed to delete identity: %w", err)
	}
	err = cli.Store.Sessions.DeleteSession(ctx, target.SignalAddress().String())
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	go cli.dispatchEvent(&events.IdentityChange{JID: target, Timestamp: time.Now(), Implicit: true})
	return nil
}

var EventAlreadyProcessed = errors.New("event was already processed")
