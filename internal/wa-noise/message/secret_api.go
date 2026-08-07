// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package message

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// DecryptReaction decifra uma reacao de grupo de anuncio de comunidade.
func DecryptReaction(ctx context.Context, t Transport, reaction *events.Message) (*waE2E.ReactionMessage, error) {
	encReaction := reaction.Message.GetEncReactionMessage()
	if encReaction == nil {
		return nil, ErrNotEncryptedReactionMessage
	}
	plaintext, err := DecryptSecret(ctx, t, reaction, EncSecretReaction, encReaction, encReaction.GetTargetMessageKey())
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

// DecryptComment decifra um comentario/resposta de grupo de anuncio de
// comunidade.
func DecryptComment(ctx context.Context, t Transport, comment *events.Message) (*waE2E.Message, error) {
	encComment := comment.Message.GetEncCommentMessage()
	if encComment == nil {
		return nil, ErrNotEncryptedCommentMessage
	}
	plaintext, err := DecryptSecret(ctx, t, comment, EncSecretComment, encComment, encComment.GetTargetMessageKey())
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

// DecryptSecretEncrypted decifra um SecretEncryptedMessage (edicao de evento,
// de enquete ou de mensagem).
//
// O mapeamento de SecretEncType -> SecretType foi copiado literalmente,
// INCLUSIVE o fato de POLL_ADD_OPTION cair em EncSecretPollEdit (e nao em
// EncSecretPollAddOption). Isso e' o que o codigo fazia antes; se for engano do
// upstream, corrigi-lo mudaria a chave derivada e quebraria a decifragem.
func DecryptSecretEncrypted(ctx context.Context, t Transport, evt *events.Message) (*waE2E.Message, error) {
	encMessage := evt.Message.GetSecretEncryptedMessage()
	if encMessage == nil {
		return nil, ErrNotSecretEncryptedMessage
	}
	var secretType SecretType
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
	plaintext, err := DecryptSecret(ctx, t, evt, secretType, encMessage, encMessage.GetTargetMessageKey())
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

// EncryptComment cifra um comentario para a mensagem raiz dada.
func EncryptComment(ctx context.Context, t Transport, rootMsgInfo *types.MessageInfo, comment *waE2E.Message) (*waE2E.Message, error) {
	plaintext, err := proto.Marshal(comment)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal comment protobuf: %w", err)
	}
	ciphertext, iv, err := EncryptSecret(ctx, t, t.OwnLID(), rootMsgInfo.Chat, rootMsgInfo.Sender, rootMsgInfo.ID, EncSecretComment, plaintext)
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

// EncryptReaction cifra uma reacao para a mensagem raiz dada.
func EncryptReaction(ctx context.Context, t Transport, rootMsgInfo *types.MessageInfo, reaction *waE2E.ReactionMessage) (*waE2E.EncReactionMessage, error) {
	reactionKey := reaction.Key
	// A chave sai do payload cifrado (vai em claro no TargetMessageKey), mas o
	// waE2E.ReactionMessage e' do chamador: sem restaurar, quem reusasse a
	// mesma struct — para reenviar, para outro chat — mandaria uma reacao sem
	// alvo. Restaurar antes de qualquer return.
	reaction.Key = nil
	defer func() { reaction.Key = reactionKey }()
	plaintext, err := proto.Marshal(reaction)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reaction protobuf: %w", err)
	}
	ciphertext, iv, err := EncryptSecret(ctx, t, t.OwnLID(), rootMsgInfo.Chat, rootMsgInfo.Sender, rootMsgInfo.ID, EncSecretReaction, plaintext)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt reaction: %w", err)
	}
	return &waE2E.EncReactionMessage{
		TargetMessageKey: reactionKey,
		EncPayload:       ciphertext,
		EncIV:            iv,
	}, nil
}
