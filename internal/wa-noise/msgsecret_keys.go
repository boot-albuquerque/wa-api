// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"wa-api/internal/wa-noise/capabilities/message"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// A derivacao de chave de segredo de mensagem vive em
// internal/wa-noise/message/ desde a Fase F/G lote 9.

// MsgSecretType e' APELIDO de tipo, e nao um tipo novo, porque internals.go
// (gerado) cita o nome antigo nas assinaturas de DangerousInternalClient e
// porque e' parte da API publica do pacote. Apelido faz dos dois o MESMO tipo,
// entao quem escrevia `whatsmeow.EncSecretPollVote` continua compilando e
// continua podendo passar o valor as funcoes de message/.
type MsgSecretType = message.SecretType

// messageEncryptedSecret e' apelido pelo mesmo motivo: internals.go o cita.
type messageEncryptedSecret = message.EncryptedSecret

// As constantes sao reexportadas por ATRIBUICAO. Elas sao entrada de HKDF: ter
// um unico dono do valor e' o que impede que a string derivadora divirja entre
// a raiz e o subpacote.
const (
	EncSecretPollVote      = message.EncSecretPollVote
	EncSecretReaction      = message.EncSecretReaction
	EncSecretComment       = message.EncSecretComment
	EncSecretReportToken   = message.EncSecretReportToken
	EncSecretEventResponse = message.EncSecretEventResponse
	EncSecretEventEdit     = message.EncSecretEventEdit
	EncSecretMessageEdit   = message.EncSecretMessageEdit
	EncSecretPollEdit      = message.EncSecretPollEdit
	EncSecretPollAddOption = message.EncSecretPollAddOption
	EncSecretBotMsg        = message.EncSecretBotMsg
)

// applyBotMessageHKDF e' chamada por send_adapter.go (o adaptador do lote 8),
// que monta o reporting token.
func applyBotMessageHKDF(messageSecret []byte) []byte {
	return message.ApplyBotMessageHKDF(messageSecret)
}

// generateMsgSecretKey e' chamada por reportingtoken.go, que continua na raiz.
func generateMsgSecretKey(
	modificationType MsgSecretType, modificationSender types.JID,
	origMsgID types.MessageID, origMsgSender types.JID, origMsgSecret []byte,
) ([]byte, []byte) {
	return message.GenerateSecretKey(modificationType, modificationSender, origMsgID, origMsgSender, origMsgSecret)
}

func getOrigSenderFromKey(msg *events.Message, key *waCommon.MessageKey) (types.JID, error) {
	return message.OrigSenderFromKey(msg, key)
}

func getKeyFromInfo(msgInfo *types.MessageInfo) *waCommon.MessageKey {
	return message.KeyFromInfo(msgInfo)
}
