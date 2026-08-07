// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"time"

	"go.mau.fi/libsignal/keys/prekey"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/send"
	"wa-api/internal/wa-noise/types"
)

// Fachadas do caminho waE2E de envio. Nenhuma tem logica: existem porque tres
// chamadores da raiz continuam citando os nomes minusculos historicos —
// internals.go (GERADO, fora do escopo do lote), retry_transport.go (o
// adaptador do lote 5) e notification_device.go/user_transport.go (o phash).
//
// Ver PATCHES.md, "Fase F/G — lote 8".

// participantListHashV2 e' chamada por notification_device.go (5 pontos) e pela
// fachada user_transport.go.ParticipantListHash, aberta no lote 7.
func participantListHashV2(participants []types.JID) string {
	return send.ParticipantListHashV2(participants)
}

func marshalMessage(to types.JID, message *waE2E.Message) (plaintext, dsmPlaintext []byte, err error) {
	return send.MarshalMessage(to, message)
}

func copyAttrs(from, to waBinary.Attrs) {
	send.CopyAttrs(from, to)
}

func (cli *Client) makeDeviceIdentityNode() waBinary.Node {
	return send.MakeDeviceIdentityNode(cli.sendT())
}

func (cli *Client) getMessageContent(
	baseNode waBinary.Node,
	message *waE2E.Message,
	msgAttrs waBinary.Attrs,
	includeIdentity bool,
	extraParams nodeExtraParams,
) []waBinary.Node {
	return send.MessageContent(cli.sendT(), baseNode, message, msgAttrs, includeIdentity, extraParams)
}

func (cli *Client) preparePeerMessageNode(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
) (*waBinary.Node, error) {
	return send.PreparePeerMessageNode(ctx, cli.sendT(), to, id, message, timings)
}

func (cli *Client) prepareMessageNode(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	participants []types.JID,
	plaintext, dsmPlaintext []byte,
	timings *MessageDebugTimings,
	extraParams nodeExtraParams,
) (*waBinary.Node, []types.JID, error) {
	return send.PrepareMessageNode(
		ctx, cli.sendT(), to, id, message, participants, plaintext, dsmPlaintext, timings, extraParams,
	)
}

func (cli *Client) encryptMessageForDevices(
	ctx context.Context,
	allDevices []types.JID,
	id string,
	msgPlaintext, dsmPlaintext []byte,
	encAttrs waBinary.Attrs,
) ([]waBinary.Node, bool, error) {
	return send.EncryptForDevices(ctx, cli.sendT(), allDevices, id, msgPlaintext, dsmPlaintext, encAttrs)
}

func (cli *Client) encryptMessageForDeviceAndWrap(
	ctx context.Context,
	plaintext []byte,
	wireIdentity,
	encryptionIdentity types.JID,
	bundle *prekey.Bundle,
	encAttrs waBinary.Attrs,
	existingSessions map[string]bool,
) (*waBinary.Node, bool, error) {
	return send.EncryptForDeviceAndWrap(
		ctx, cli.sendT(), plaintext, wireIdentity, encryptionIdentity, bundle, encAttrs, existingSessions,
	)
}

func (cli *Client) encryptMessageForDevice(
	ctx context.Context,
	plaintext []byte,
	to types.JID,
	bundle *prekey.Bundle,
	extraAttrs waBinary.Attrs,
	existingSessions map[string]bool,
) (*waBinary.Node, bool, error) {
	return send.EncryptForDevice(ctx, cli.sendT(), plaintext, to, bundle, extraAttrs, existingSessions)
}

func (cli *Client) sendNewsletter(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	mediaID string,
	timings *MessageDebugTimings,
) ([]byte, error) {
	return send.Newsletter(ctx, cli.sendT(), to, id, message, mediaID, timings)
}

func (cli *Client) sendGroup(
	ctx context.Context,
	ownID,
	to types.JID,
	participants []types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
	extraParams nodeExtraParams,
) (string, []byte, error) {
	return send.Group(ctx, cli.sendT(), ownID, to, participants, id, message, timings, extraParams)
}

func (cli *Client) sendPeerMessage(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
) ([]byte, error) {
	return send.PeerMessage(ctx, cli.sendT(), to, id, message, timings)
}

func (cli *Client) sendDM(
	ctx context.Context,
	ownID,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
	extraParams nodeExtraParams,
) (string, []byte, error) {
	return send.DM(ctx, cli.sendT(), ownID, to, id, message, timings, extraParams)
}

func (cli *Client) awaitSendAck(
	ctx context.Context,
	req *SendRequestExtra,
	resp *SendResponse,
	respChan chan *waBinary.Node,
	data []byte,
	start time.Time,
) (*waBinary.Node, error) {
	return send.AwaitAck(ctx, cli.sendT(), req, resp, respChan, data, start)
}

func (cli *Client) applySendAck(respNode *waBinary.Node, to types.JID, phash string, resp *SendResponse) error {
	return send.ApplyAck(cli.sendT(), respNode, to, phash, resp)
}

func (cli *Client) invalidateParticipantCache(to types.JID) {
	send.InvalidateParticipantCache(cli.sendT(), to)
}
