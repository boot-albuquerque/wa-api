// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/msgattrs"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/send"
	"wa-api/internal/wa-noise/protocol/types"
)

// Fachadas do caminho v3/FB de envio, pelo mesmo motivo das do waE2E: os nomes
// minusculos historicos continuam citados por internals.go (GERADO) e por
// retry_transport.go. Ver PATCHES.md, "Fase F/G — lote 8".

func (cli *Client) sendGroupV3(
	ctx context.Context,
	to,
	ownID types.JID,
	id types.MessageID,
	messageApp []byte,
	msgAttrs msgattrs.MessageAttrs,
	frankingTag []byte,
	timings *MessageDebugTimings,
) (string, []byte, error) {
	return send.GroupV3(ctx, cli.sendT(), to, ownID, id, messageApp, msgAttrs, frankingTag, timings)
}

func (cli *Client) sendDMV3(
	ctx context.Context,
	to,
	ownID types.JID,
	id types.MessageID,
	messageApp []byte,
	msgAttrs msgattrs.MessageAttrs,
	frankingTag []byte,
	timings *MessageDebugTimings,
) ([]byte, string, error) {
	return send.DMV3(ctx, cli.sendT(), to, ownID, id, messageApp, msgAttrs, frankingTag, timings)
}

func (cli *Client) prepareMessageNodeV3(
	ctx context.Context,
	to,
	ownID types.JID,
	id types.MessageID,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	msgAttrs msgattrs.MessageAttrs,
	frankingTag []byte,
	participants []types.JID,
	timings *MessageDebugTimings,
) (*waBinary.Node, []types.JID, error) {
	return send.PrepareMessageNodeV3(
		ctx, cli.sendT(), to, ownID, id, payload, skdm, msgAttrs, frankingTag, participants, timings,
	)
}

func (cli *Client) encryptMessageForDevicesV3(
	ctx context.Context,
	allDevices []types.JID,
	ownID types.JID,
	id string,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	encAttrs waBinary.Attrs,
) ([]waBinary.Node, error) {
	return send.EncryptForDevicesV3(ctx, cli.sendT(), allDevices, ownID, id, payload, skdm, dsm, encAttrs)
}

func (cli *Client) encryptMessageForDeviceAndWrapV3(
	ctx context.Context,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	return send.EncryptForDeviceAndWrapV3(ctx, cli.sendT(), payload, skdm, dsm, to, bundle, encAttrs)
}

func (cli *Client) encryptMessageForDeviceV3(
	ctx context.Context,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	dsm *waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
) (*waBinary.Node, error) {
	return send.EncryptForDeviceV3(ctx, cli.sendT(), payload, skdm, dsm, to, bundle, extraAttrs)
}
