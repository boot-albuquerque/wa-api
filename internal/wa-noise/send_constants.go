// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import "wa-api/internal/wa-noise/send"

// As constantes de wire do stanza <message> moram em
// internal/wa-noise/send/constants.go desde a Fase F/G lote 8. As nove abaixo
// continuam existindo na raiz porque o caminho de ENTRADA (message_decrypt.go,
// message_decrypt_session.go, message.go) e o de segredo de mensagem
// (msgsecret_poll.go) as leem de volta — e por atribuicao, nao por
// redeclaracao, para que o valor que vai para o fio tenha UM dono. Redeclarar
// `encTypeMsg = "msg"` dos dois lados da fronteira e' exatamente a divergencia
// silenciosa que a Fase F/G existe para impedir.

// Tag e atributos do <enc>, lidos de volta pelo caminho de decifragem.
const (
	encNodeTag         = send.EncNodeTag
	encAttrVersion     = send.EncAttrVersion
	encAttrType        = send.EncAttrType
	encAttrDecryptFail = send.EncAttrDecryptFail
)

// Valores do atributo `type` do <enc>, comparados em message_decrypt.go.
const (
	encTypeMsg       = send.EncTypeMsg
	encTypePreKeyMsg = send.EncTypePreKeyMsg
	encTypeSenderKey = send.EncTypeSenderKey
)

// msgCategoryPeer e' comparado em message.go ao classificar a mensagem
// recebida; messageSecretSize e' o tamanho do segredo gerado em
// msgsecret_poll.go.
const (
	msgCategoryPeer   = send.MsgCategoryPeer
	messageSecretSize = send.MessageSecretSize
)
