package core

import (
	"wa-api/internal/noise/capabilities/group"
	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// Fachada; ver o cabecalho de group.go. So' parseGroupNode sobrou aqui: as
// outras tres funcoes de parsing de group_parse.go (parseParticipant,
// parseGroupLinkTargetNode, parseParticipantList) nao tinham receptor `cli`,
// entao internals.go nunca as citou e nao ha' contrato a preservar — quem
// precisa delas hoje chama group.ParseParticipant, group.ParseLinkTargetNode e
// group.ParseParticipantList diretamente.

func (cli *Client) parseGroupNode(groupNode *waBinary.Node) (*types.GroupInfo, error) {
	return group.ParseNode(cli.groupT(), groupNode)
}
