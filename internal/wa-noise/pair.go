// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/capabilities/pairing"
	"wa-api/internal/wa-noise/protocol/types"
)

// A logica deste dominio vive em internal/wa-noise/pairing/. O que sobra aqui
// sao fachadas: elas guardam o contrato historico (nomes, assinaturas e o
// receptor *Client) e delegam. Ver PATCHES.md, "Fase F/G — lote 4".

// handleIQ e' o nodeHandler de <iq> registrado em client.go. Continua na raiz
// porque e' entrada do despachante de nos, nao logica de pareamento.
func (cli *Client) handleIQ(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	children := node.GetChildren()
	if len(children) != 1 || node.Attrs["from"] != types.ServerJID {
		return
	}
	switch children[0].Tag {
	case "pair-device":
		cli.handlePairDevice(ctx, node)
	case "pair-success":
		cli.handlePairSuccess(ctx, node)
	}
}

func (cli *Client) handlePairDevice(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	pairing.HandleDeviceNode(ctx, cli.pairT(), node)
}

func (cli *Client) getQRClientType() PairClientType {
	if cli == nil {
		return pairing.DetectClientType("")
	}
	return pairing.DetectClientType(cli.QRClientType)
}

func (cli *Client) makeQRData(ref []byte, clientType PairClientType) string {
	if cli == nil {
		return ""
	}
	return pairing.MakeQRData(cli.pairT(), ref, clientType)
}

func (cli *Client) handlePairSuccess(ctx context.Context, node *waBinary.Node) {
	if cli == nil {
		return
	}
	pairing.HandleSuccessNode(ctx, cli.pairT(), node)
}

func (cli *Client) handlePair(ctx context.Context, deviceIdentityBytes []byte, reqID, businessName, platform string, jid, lid types.JID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	return pairing.Confirm(ctx, cli.pairT(), deviceIdentityBytes, reqID, businessName, platform, jid, lid)
}

func (cli *Client) sendPairError(ctx context.Context, id string, code int, text string) {
	if cli == nil {
		return
	}
	pairing.SendError(ctx, cli.pairT(), id, code, text)
}
