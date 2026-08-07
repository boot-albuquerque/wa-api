// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/protocol/types"
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
