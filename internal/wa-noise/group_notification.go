// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types/events"
)

// Fachada; ver o cabecalho de group.go. Os quatro metodos abaixo sao citados
// por internals.go (gerado) e o ultimo e' chamado por notification.go.

func (cli *Client) parseGroupCreate(parentNode, node *waBinary.Node) (*events.JoinedGroup, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	return group.ParseCreate(cli.groupT(), parentNode, node)
}

func (cli *Client) parseGroupChange(node *waBinary.Node) (*events.GroupInfo, []store.LIDMapping, error) {
	return group.ParseChange(cli.groupT(), node)
}

func (cli *Client) updateGroupParticipantCache(evt *events.GroupInfo) {
	group.UpdateParticipantCache(cli.groupT(), evt)
}

func (cli *Client) parseGroupNotification(node *waBinary.Node) (any, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	return group.ParseNotification(cli.groupT(), node)
}
