// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/wa-noise/proto/waCommon"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/util/hkdfutil"
)

type MsgSecretType string

const (
	EncSecretPollVote      MsgSecretType = "Poll Vote"
	EncSecretReaction      MsgSecretType = "Enc Reaction"
	EncSecretComment       MsgSecretType = "Enc Comment"
	EncSecretReportToken   MsgSecretType = "Report Token"
	EncSecretEventResponse MsgSecretType = "Event Response"
	EncSecretEventEdit     MsgSecretType = "Event Edit"
	EncSecretMessageEdit   MsgSecretType = "Message Edit"
	EncSecretPollEdit      MsgSecretType = "Poll Edit"
	EncSecretPollAddOption MsgSecretType = "Poll Add Option"
	EncSecretBotMsg        MsgSecretType = "Bot Message"
)

func applyBotMessageHKDF(messageSecret []byte) []byte {
	return hkdfutil.SHA256(messageSecret, nil, []byte(EncSecretBotMsg), msgSecretKeyLength)
}

func generateMsgSecretKey(
	modificationType MsgSecretType, modificationSender types.JID,
	origMsgID types.MessageID, origMsgSender types.JID, origMsgSecret []byte,
) ([]byte, []byte) {
	origMsgSenderStr := origMsgSender.ToNonAD().String()
	modificationSenderStr := modificationSender.ToNonAD().String()

	useCaseSecret := make([]byte, 0, len(origMsgID)+len(origMsgSenderStr)+len(modificationSenderStr)+len(modificationType))
	useCaseSecret = append(useCaseSecret, origMsgID...)
	useCaseSecret = append(useCaseSecret, origMsgSenderStr...)
	useCaseSecret = append(useCaseSecret, modificationSenderStr...)
	useCaseSecret = append(useCaseSecret, modificationType...)

	secretKey := hkdfutil.SHA256(origMsgSecret, nil, useCaseSecret, msgSecretKeyLength)
	var additionalData []byte
	switch modificationType {
	case EncSecretPollVote, EncSecretEventResponse, "":
		additionalData = fmt.Appendf(nil, "%s\x00%s", origMsgID, modificationSenderStr)
	}

	return secretKey, additionalData
}

// errUnexpectedOrigSenderServer indica que o participante da MessageKey do
// grupo nao esta' nem em @s.whatsapp.net nem em @lid — servidor que este
// pacote nao sabe usar como remetente original.
var errUnexpectedOrigSenderServer = errors.New("unexpected server")

func getOrigSenderFromKey(msg *events.Message, key *waCommon.MessageKey) (types.JID, error) {
	if key.GetFromMe() {
		// fromMe always means the poll and vote were sent by the same user
		// TODO this is wrong if the message key used @s.whatsapp.net, but the new event is from @lid
		return msg.Info.Sender, nil
	} else if msg.Info.Chat.Server == types.DefaultUserServer || msg.Info.Chat.Server == types.HiddenUserServer {
		sender, err := types.ParseJID(key.GetRemoteJID())
		if err != nil {
			return types.EmptyJID, fmt.Errorf("failed to parse JID %q of original message sender: %w", key.GetRemoteJID(), err)
		}
		return sender, nil
	} else {
		// A ordem importa: antes o erro do ParseJID era sobrescrito pelo
		// "unexpected server" derivado do JID zerado que ele devolve, e a causa
		// real do parse quebrado nunca chegava ao log.
		sender, err := types.ParseJID(key.GetParticipant())
		if err != nil {
			return types.EmptyJID, fmt.Errorf("failed to parse JID %q of original message sender: %w", key.GetParticipant(), err)
		}
		if sender.Server != types.DefaultUserServer && sender.Server != types.HiddenUserServer {
			return types.EmptyJID, fmt.Errorf("failed to parse JID %q of original message sender: %w", key.GetParticipant(), errUnexpectedOrigSenderServer)
		}
		return sender, nil
	}
}

type messageEncryptedSecret interface {
	GetEncIV() []byte
	GetEncPayload() []byte
}

func getKeyFromInfo(msgInfo *types.MessageInfo) *waCommon.MessageKey {
	creationKey := &waCommon.MessageKey{
		RemoteJID: proto.String(msgInfo.Chat.String()),
		FromMe:    proto.Bool(msgInfo.IsFromMe),
		ID:        proto.String(msgInfo.ID),
	}
	if msgInfo.IsGroup {
		creationKey.Participant = proto.String(msgInfo.Sender.String())
	}
	return creationKey
}
