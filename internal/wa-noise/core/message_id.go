package wanoise

import (
	"wa-api/internal/wa-noise/capabilities/message"
	"wa-api/internal/wa-noise/protocol/types"
)

// WebMessageIDPrefix e' reexportado por ATRIBUICAO a partir de message/, e nao
// redeclarado: e' prefixo de fio, e group_transport.go tambem o le. Um so' dono
// do valor.
const WebMessageIDPrefix = message.WebMessageIDPrefix

// GenerateMessageID generates a random string that can be used as a message ID on WhatsApp.
//
//	msgID := cli.GenerateMessageID()
//	cli.SendMessage(context.Background(), targetJID, &waE2E.Message{...}, wa-noise.SendRequestExtra{ID: msgID})
//
// O receptor nil continua sendo valido, como antes da extracao: getOwnID
// devolve EmptyJID e a checagem de MessengerConfig e' guardada por `cli != nil`.
func (cli *Client) GenerateMessageID() types.MessageID {
	return message.GenerateID(cli.getOwnID(), cli != nil && cli.MessengerConfig != nil)
}

// GenerateFacebookMessageID gera o ID de mensagem numerico do Messenger.
func GenerateFacebookMessageID() int64 {
	return message.GenerateFacebookID()
}

// GenerateMessageID generates a random string that can be used as a message ID on WhatsApp.
//
//	msgID := wa-noise.GenerateMessageID()
//	cli.SendMessage(context.Background(), targetJID, &waE2E.Message{...}, wa-noise.SendRequestExtra{ID: msgID})
//
// Deprecated: WhatsApp web has switched to using a hash of the current timestamp, user id and random bytes. Use Client.GenerateMessageID instead.
func GenerateMessageID() types.MessageID {
	return message.GenerateLegacyID()
}
