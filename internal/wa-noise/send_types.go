// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import "wa-api/internal/wa-noise/send"

// Os quatro tipos do dominio de envio moram em internal/wa-noise/send desde a
// Fase F/G lote 8. A raiz os reexporta por APELIDO DE TIPO, nao por definicao
// nova: o apelido faz dos dois o MESMO tipo, entao todo chamador externo que
// escrevia whatsmeow.SendResponse{...} continua compilando, e
// MessageDebugTimings continua satisfazendo zerolog.LogObjectMarshaler com o
// mesmo metodo. Uma definicao nova seria um tipo distinto e quebraria a API.

// SendResponse is the result of a message send. See send.Response.
type SendResponse = send.Response

// SendRequestExtra contains the optional parameters for SendMessage.
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
type SendRequestExtra = send.RequestExtra

// MessageDebugTimings is the message handling duration breakdown of a send.
type MessageDebugTimings = send.DebugTimings

// nodeExtraParams continua com o nome minusculo porque internals.go, que e'
// GERADO e esta' fora do escopo do lote, o cita em quatro assinaturas de
// DangerousInternalClient.
type nodeExtraParams = send.NodeExtraParams
