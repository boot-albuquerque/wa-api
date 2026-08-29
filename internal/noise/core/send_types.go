package core

import "wa-api/internal/noise/capabilities/send"

// Os quatro tipos do dominio de envio moram em internal/wa-noise/send desde a
// Fase F/G lote 8. A raiz os reexporta por APELIDO DE TIPO, nao por definicao
// nova: o apelido faz dos dois o MESMO tipo, entao todo chamador externo que
// escrevia wa-noise.SendResponse{...} continua compilando, e
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
//	cli.SendMessage(ctx, to, message, wa-noise.SendRequestExtra{...})
//
// Trying to add multiple extra parameters will return an error.
type SendRequestExtra = send.RequestExtra

// MessageDebugTimings is the message handling duration breakdown of a send.
type MessageDebugTimings = send.DebugTimings

// nodeExtraParams continua com o nome minusculo porque internals.go, que e'
// GERADO e esta' fora do escopo do lote, o cita em quatro assinaturas de
// DangerousInternalClient.
type nodeExtraParams = send.NodeExtraParams
