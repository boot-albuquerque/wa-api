// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// Response is the result of a message send.
//
// A raiz a reexporta como whatsmeow.SendResponse por APELIDO DE TIPO
// (`type SendResponse = send.Response`), nao por definicao de tipo novo: o
// apelido faz dos dois o MESMO tipo, entao todo chamador externo que ja'
// escrevia `whatsmeow.SendResponse{...}` ou fazia type switch continua
// compilando e casando. Uma definicao nova seria um tipo distinto e quebraria
// a API publica.
type Response struct {
	// The message timestamp returned by the server
	Timestamp time.Time

	// The ID of the sent message
	ID types.MessageID

	// The server-specified ID of the sent message. Only present for newsletter messages.
	ServerID types.MessageServerID

	// Message handling duration, used for debugging
	DebugTimings DebugTimings

	// The identity the message was sent with (LID or PN)
	// This is currently not reliable in all cases.
	Sender types.JID
}

// RequestExtra contains the optional parameters for SendMessage.
//
// By default, optional parameters don't have to be provided at all, e.g.
//
//	cli.SendMessage(ctx, to, message)
//
// When providing optional parameters, add a single instance of this struct as the last parameter:
//
//	cli.SendMessage(ctx, to, message, whatsmeow.SendRequestExtra{...})
//
// Trying to add multiple extra parameters will return an error.
//
// Reexportada pela raiz como whatsmeow.SendRequestExtra, por apelido de tipo —
// ver o doc de Response.
type RequestExtra struct {
	// The message ID to use when sending. If this is not provided, a random message ID will be generated
	ID types.MessageID
	// JID of the bot to be invoked (optional)
	InlineBotJID types.JID
	// Should the message be sent as a peer message (protocol messages to your own devices, e.g. app state key requests)
	Peer bool
	// A timeout for the send request. Unlike timeouts using the context parameter, this only applies
	// to the actual response waiting and not preparing/encrypting the message.
	// Defaults to 75 seconds. The timeout can be disabled by using a negative value.
	Timeout time.Duration
	// When sending media to newsletters, the Handle field returned by the file upload.
	MediaHandle string

	Meta *types.MsgMetaInfo
	// use this only if you know what you are doing
	AdditionalNodes *[]waBinary.Node
}

// NodeExtraParams reune os filhos opcionais do stanza que sao decididos no
// preparo e consumidos na montagem do no.
//
// Era o nodeExtraParams (minusculo) da raiz. E' EXPORTADA aqui — e a raiz
// mantem `type nodeExtraParams = send.NodeExtraParams` — porque internals.go,
// que e' GERADO e esta' fora do escopo deste lote, cita o nome minusculo nas
// assinaturas de quatro metodos de DangerousInternalClient. O apelido preserva
// essas assinaturas byte a byte.
//
// Os CAMPOS continuam minusculos: nenhum codigo fora deste pacote os le ou
// escreve (internals.go so' repassa o valor, retry_transport.go so' passa o
// zero), entao exporta-los seria alargar a superficie sem chamador.
type NodeExtraParams struct {
	botNode         *waBinary.Node
	metaNode        *waBinary.Node
	additionalNodes *[]waBinary.Node
	addressingMode  types.AddressingMode
}
