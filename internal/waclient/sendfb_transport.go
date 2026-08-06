// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/protocol"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/proto/waCommon"
	"wa-api/internal/waclient/proto/waMsgTransport"
	"wa-api/internal/waclient/types"
)

func (cli *Client) sendGroupV3(
	ctx context.Context,
	to,
	ownID types.JID,
	id types.MessageID,
	messageApp []byte,
	msgAttrs messageAttrs,
	frankingTag []byte,
	timings *MessageDebugTimings,
) (string, []byte, error) {
	var groupMeta *groupMetaCache
	var err error
	start := time.Now()
	if to.Server == types.GroupServer {
		groupMeta, err = cli.getCachedGroupData(ctx, to)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get group members: %w", err)
		}
	}
	timings.GetParticipants = time.Since(start)

	start = time.Now()
	builder := groups.NewGroupSessionBuilder(cli.Store, pbSerializer)
	senderKeyName := protocol.NewSenderKeyName(to.String(), ownID.SignalAddress())
	signalSKDMessage, err := builder.Create(ctx, senderKeyName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create sender key distribution message to send %s to %s: %w", id, to, err)
	}
	skdm := &waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage{
		GroupID:                             proto.String(to.String()),
		AxolotlSenderKeyDistributionMessage: signalSKDMessage.Serialize(),
	}

	cipher := groups.NewGroupCipher(builder, senderKeyName, cli.Store)
	plaintext, err := proto.Marshal(&waMsgTransport.MessageTransport{
		Payload: &waMsgTransport.MessageTransport_Payload{
			ApplicationPayload: &waCommon.SubProtocol{
				Payload: messageApp,
				Version: proto.Int32(FBMessageApplicationVersion),
			},
			FutureProof: waCommon.FutureProofBehavior_PLACEHOLDER.Enum(),
		},
		Protocol: &waMsgTransport.MessageTransport_Protocol{
			Integral: &waMsgTransport.MessageTransport_Protocol_Integral{
				Padding: padMessage(nil),
				DSM:     nil,
			},
			Ancillary: &waMsgTransport.MessageTransport_Protocol_Ancillary{
				Skdm:               nil,
				DeviceListMetadata: nil,
				Icdc:               nil,
				BackupDirective: &waMsgTransport.MessageTransport_Protocol_Ancillary_BackupDirective{
					MessageID:  &id,
					ActionType: waMsgTransport.MessageTransport_Protocol_Ancillary_BackupDirective_UPSERT.Enum(),
				},
			},
		},
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal message transport: %w", err)
	}
	encrypted, err := cipher.Encrypt(ctx, plaintext)
	if err != nil {
		return "", nil, fmt.Errorf("failed to encrypt group message to send %s to %s: %w", id, to, err)
	}
	ciphertext := encrypted.SignedSerialize()
	timings.GroupEncrypt = time.Since(start)

	node, allDevices, err := cli.prepareMessageNodeV3(
		ctx, to, ownID, id, nil, skdm, msgAttrs, frankingTag, groupMeta.Members, timings,
	)
	if err != nil {
		return "", nil, err
	}

	phash := participantListHashV2(allDevices)
	node.Attrs["phash"] = phash
	skMsg := waBinary.Node{
		Tag:     "enc",
		Content: ciphertext,
		Attrs:   waBinary.Attrs{"v": "3", "type": "skmsg"},
	}
	if msgAttrs.MediaType != "" {
		skMsg.Attrs["mediatype"] = msgAttrs.MediaType
	}
	node.Content = append(node.GetChildren(), skMsg)

	start = time.Now()
	data, err := cli.sendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return phash, data, nil
}

func (cli *Client) sendDMV3(
	ctx context.Context,
	to,
	ownID types.JID,
	id types.MessageID,
	messageApp []byte,
	msgAttrs messageAttrs,
	frankingTag []byte,
	timings *MessageDebugTimings,
) ([]byte, string, error) {
	payload := &waMsgTransport.MessageTransport_Payload{
		ApplicationPayload: &waCommon.SubProtocol{
			Payload: messageApp,
			Version: proto.Int32(FBMessageApplicationVersion),
		},
		FutureProof: waCommon.FutureProofBehavior_PLACEHOLDER.Enum(),
	}

	node, allDevices, err := cli.prepareMessageNodeV3(ctx, to, ownID, id, payload, nil, msgAttrs, frankingTag, []types.JID{to, ownID.ToNonAD()}, timings)
	if err != nil {
		return nil, "", err
	}
	start := time.Now()
	data, err := cli.sendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return nil, "", fmt.Errorf("failed to send message node: %w", err)
	}
	return data, participantListHashV2(allDevices), nil
}

func (cli *Client) prepareMessageNodeV3(
	ctx context.Context,
	to,
	ownID types.JID,
	id types.MessageID,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	msgAttrs messageAttrs,
	frankingTag []byte,
	participants []types.JID,
	timings *MessageDebugTimings,
) (*waBinary.Node, []types.JID, error) {
	start := time.Now()
	allDevices, err := cli.GetUserDevices(ctx, participants)
	timings.GetDevices = time.Since(start)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get device list: %w", err)
	}

	encAttrs := waBinary.Attrs{}
	attrs := waBinary.Attrs{
		"id":   id,
		"type": msgAttrs.Type,
		"to":   to,
	}
	// Only include mediatype on DMs, for groups it's in the skmsg node
	if payload != nil && msgAttrs.MediaType != "" {
		encAttrs["mediatype"] = msgAttrs.MediaType
	}
	if msgAttrs.Edit != "" {
		attrs["edit"] = string(msgAttrs.Edit)
	}
	if msgAttrs.DecryptFail != "" {
		encAttrs["decrypt-fail"] = string(msgAttrs.DecryptFail)
	}

	dsm := &waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage{
		DestinationJID: proto.String(to.String()),
		Phash:          proto.String(""),
	}

	start = time.Now()
	participantNodes, err := cli.encryptMessageForDevicesV3(ctx, allDevices, ownID, id, payload, skdm, dsm, encAttrs)
	if err != nil {
		return nil, nil, err
	}
	timings.PeerEncrypt = time.Since(start)
	content := make([]waBinary.Node, 0, 4)
	content = append(content, waBinary.Node{
		Tag:     "participants",
		Content: participantNodes,
	})
	metaAttrs := make(waBinary.Attrs)
	if msgAttrs.PollType != "" {
		metaAttrs["polltype"] = msgAttrs.PollType
	}
	if msgAttrs.DecryptFail != "" {
		metaAttrs["decrypt-fail"] = string(msgAttrs.DecryptFail)
	}
	if len(metaAttrs) > 0 {
		content = append(content, waBinary.Node{
			Tag:   "meta",
			Attrs: metaAttrs,
		})
	}
	traceRequestID := uuid.New()
	content = append(content, waBinary.Node{
		Tag: "franking",
		Content: []waBinary.Node{{
			Tag:     "franking_tag",
			Content: frankingTag,
		}},
	}, waBinary.Node{
		Tag: "trace",
		Content: []waBinary.Node{{
			Tag:     "request_id",
			Content: traceRequestID[:],
		}},
	})
	return &waBinary.Node{
		Tag:     "message",
		Attrs:   attrs,
		Content: content,
	}, allDevices, nil
}
