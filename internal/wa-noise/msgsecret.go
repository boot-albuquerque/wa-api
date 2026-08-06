// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"strings"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/util/gcmutil"
)

func (cli *Client) decryptMsgSecret(ctx context.Context, msg *events.Message, useCase MsgSecretType, encrypted messageEncryptedSecret, origMsgKey *waCommon.MessageKey) ([]byte, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	origSender, err := getOrigSenderFromKey(msg, origMsgKey)
	if err != nil {
		return nil, err
	}
	baseEncKey, storedOrigSender, err := cli.Store.MsgSecrets.GetMessageSecret(ctx, msg.Info.Chat, origSender, origMsgKey.GetID())
	if err != nil {
		return nil, fmt.Errorf("failed to get original message secret key: %w", err)
	}
	if baseEncKey == nil {
		return nil, ErrOriginalMessageSecretNotFound
	}
	secretKey, additionalData := generateMsgSecretKey(useCase, msg.Info.Sender, origMsgKey.GetID(), origSender, baseEncKey)
	plaintext, err := gcmutil.Decrypt(secretKey, encrypted.GetEncIV(), encrypted.GetEncPayload(), additionalData)
	if err != nil {
		// Hack for trying both the original sender in the new message and the one who we received the secret key from.
		// This will hopefully become unnecessary when WhatsApp fully finishes their migration to LIDs.
		if origSender != storedOrigSender && strings.Contains(err.Error(), "message authentication failed") {
			secretKey, additionalData = generateMsgSecretKey(useCase, msg.Info.Sender, origMsgKey.GetID(), storedOrigSender, baseEncKey)
			plaintext, err = gcmutil.Decrypt(secretKey, encrypted.GetEncIV(), encrypted.GetEncPayload(), additionalData)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret message: %w (sender: %s, orig sender: %s and %s)", err, msg.Info.Sender, origSender, storedOrigSender)
		}
	}
	return plaintext, nil
}

func (cli *Client) encryptMsgSecret(ctx context.Context, ownID, chat, origSender types.JID, origMsgID types.MessageID, useCase MsgSecretType, plaintext []byte) (ciphertext, iv []byte, err error) {
	if cli == nil {
		return nil, nil, ErrClientIsNil
	} else if ownID.IsEmpty() {
		return nil, nil, ErrNotLoggedIn
	}

	baseEncKey, origSender, err := cli.Store.MsgSecrets.GetMessageSecret(ctx, chat, origSender, origMsgID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get original message secret key: %w", err)
	} else if baseEncKey == nil {
		return nil, nil, ErrOriginalMessageSecretNotFound
	}
	secretKey, additionalData := generateMsgSecretKey(useCase, ownID, origMsgID, origSender, baseEncKey)

	iv = random.Bytes(12)
	ciphertext, err = gcmutil.Encrypt(secretKey, iv, plaintext, additionalData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encrypt secret message: %w", err)
	}
	return ciphertext, iv, nil
}

func (cli *Client) decryptBotMessage(ctx context.Context, messageSecret []byte, msMsg messageEncryptedSecret, messageID types.MessageID, targetSenderJID types.JID, info *types.MessageInfo) ([]byte, error) {
	newKey, additionalData := generateMsgSecretKey("", info.Sender, messageID, targetSenderJID, applyBotMessageHKDF(messageSecret))

	plaintext, err := gcmutil.Decrypt(newKey, msMsg.GetEncIV(), msMsg.GetEncPayload(), additionalData)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt secret message: %w", err)
	}

	return plaintext, nil
}

// DecryptReaction decrypts a reaction message in a community announcement group.
//
//	if evt.Message.GetEncReactionMessage() != nil {
//		reaction, err := cli.DecryptReaction(evt)
//		if err != nil {
//			fmt.Println(":(", err)
//			return
//		}
//		fmt.Printf("Reaction message: %+v\n", reaction)
//	}
func (cli *Client) DecryptReaction(ctx context.Context, reaction *events.Message) (*waE2E.ReactionMessage, error) {
	encReaction := reaction.Message.GetEncReactionMessage()
	if encReaction == nil {
		return nil, ErrNotEncryptedReactionMessage
	}
	plaintext, err := cli.decryptMsgSecret(ctx, reaction, EncSecretReaction, encReaction, encReaction.GetTargetMessageKey())
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt reaction: %w", err)
	}
	var msg waE2E.ReactionMessage
	err = proto.Unmarshal(plaintext, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode reaction protobuf: %w", err)
	}
	return &msg, nil
}

// DecryptComment decrypts a reply/comment message in a community announcement group.
//
//	if evt.Message.GetEncCommentMessage() != nil {
//		comment, err := cli.DecryptComment(evt)
//		if err != nil {
//			fmt.Println(":(", err)
//			return
//		}
//		fmt.Printf("Comment message: %+v\n", comment)
//	}
func (cli *Client) DecryptComment(ctx context.Context, comment *events.Message) (*waE2E.Message, error) {
	encComment := comment.Message.GetEncCommentMessage()
	if encComment == nil {
		return nil, ErrNotEncryptedCommentMessage
	}
	plaintext, err := cli.decryptMsgSecret(ctx, comment, EncSecretComment, encComment, encComment.GetTargetMessageKey())
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt comment: %w", err)
	}
	var msg waE2E.Message
	err = proto.Unmarshal(plaintext, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode comment protobuf: %w", err)
	}
	return &msg, nil
}

func (cli *Client) DecryptSecretEncryptedMessage(ctx context.Context, evt *events.Message) (*waE2E.Message, error) {
	encMessage := evt.Message.GetSecretEncryptedMessage()
	if encMessage == nil {
		return nil, ErrNotSecretEncryptedMessage
	}
	var secretType MsgSecretType
	switch encMessage.GetSecretEncType() {
	case waE2E.SecretEncryptedMessage_EVENT_EDIT:
		secretType = EncSecretEventEdit
	case waE2E.SecretEncryptedMessage_POLL_EDIT:
		secretType = EncSecretPollEdit
	case waE2E.SecretEncryptedMessage_POLL_ADD_OPTION:
		secretType = EncSecretPollEdit
	case waE2E.SecretEncryptedMessage_MESSAGE_EDIT:
		secretType = EncSecretMessageEdit
	default:
		return nil, fmt.Errorf("unsupported secret enc type: %s", encMessage.SecretEncType.String())
	}
	plaintext, err := cli.decryptMsgSecret(ctx, evt, secretType, encMessage, encMessage.GetTargetMessageKey())
	if err != nil {
		return nil, err
	}
	var msg waE2E.Message
	err = proto.Unmarshal(plaintext, &msg)
	if err != nil {
		return nil, fmt.Errorf("failed to decode message protobuf: %w", err)
	}
	if evt.Message.MessageContextInfo != nil && msg.MessageContextInfo == nil {
		msg.MessageContextInfo = evt.Message.MessageContextInfo
	}
	return &msg, nil
}

func (cli *Client) EncryptComment(ctx context.Context, rootMsgInfo *types.MessageInfo, comment *waE2E.Message) (*waE2E.Message, error) {
	plaintext, err := proto.Marshal(comment)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal comment protobuf: %w", err)
	}
	ciphertext, iv, err := cli.encryptMsgSecret(ctx, cli.getOwnLID(), rootMsgInfo.Chat, rootMsgInfo.Sender, rootMsgInfo.ID, EncSecretComment, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt comment: %w", err)
	}
	return &waE2E.Message{
		EncCommentMessage: &waE2E.EncCommentMessage{
			TargetMessageKey: &waCommon.MessageKey{
				RemoteJID:   proto.String(rootMsgInfo.Chat.String()),
				Participant: proto.String(rootMsgInfo.Sender.ToNonAD().String()),
				FromMe:      proto.Bool(rootMsgInfo.IsFromMe),
				ID:          proto.String(rootMsgInfo.ID),
			},
			EncPayload: ciphertext,
			EncIV:      iv,
		},
	}, nil
}

func (cli *Client) EncryptReaction(ctx context.Context, rootMsgInfo *types.MessageInfo, reaction *waE2E.ReactionMessage) (*waE2E.EncReactionMessage, error) {
	reactionKey := reaction.Key
	reaction.Key = nil
	plaintext, err := proto.Marshal(reaction)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reaction protobuf: %w", err)
	}
	ciphertext, iv, err := cli.encryptMsgSecret(ctx, cli.getOwnLID(), rootMsgInfo.Chat, rootMsgInfo.Sender, rootMsgInfo.ID, EncSecretReaction, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt reaction: %w", err)
	}
	return &waE2E.EncReactionMessage{
		TargetMessageKey: reactionKey,
		EncPayload:       ciphertext,
		EncIV:            iv,
	}, nil
}
