package whatsmeow

import (
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/message"
	"wa-api/internal/wa-noise/protocol/types"
)

// O parsing de stanza <message> vive em internal/wa-noise/message/ desde a Fase
// F/G lote 9. As fachadas abaixo existem porque internals.go (gerado) cita os
// quatro nomes minusculos e porque presence.go e receipt.go chamam
// parseMessageSource.

func (cli *Client) parseMessageSource(node *waBinary.Node, requireParticipant bool) (types.MessageSource, error) {
	return message.ParseSource(cli.msgT(), node, requireParticipant)
}

func (cli *Client) parseMsgBotInfo(node waBinary.Node) (types.MsgBotInfo, error) {
	return message.ParseBotInfo(node)
}

func (cli *Client) parseMsgMetaInfo(node waBinary.Node) (types.MsgMetaInfo, error) {
	return message.ParseMetaInfo(node)
}

func (cli *Client) parseMessageInfo(node *waBinary.Node) (*types.MessageInfo, error) {
	return message.ParseInfo(cli.msgT(), node)
}
