// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mau.fi/libsignal/groups"
	"go.mau.fi/libsignal/protocol"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/group"
	"wa-api/internal/wa-noise/msgattrs"
	"wa-api/internal/wa-noise/msgpad"
	"wa-api/internal/wa-noise/protocol/proto/waCommon"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/retry"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

// FBApplicationVersion e' o `version` do SubProtocol de aplicacao FB. O valor
// mora em internal/wa-noise/retry desde a Fase F/G lote 5, para que o caminho
// de retry e o de envio normal nao possam divergir; a raiz o reexporta como
// whatsmeow.FBMessageApplicationVersion.
const FBApplicationVersion = retry.FBApplicationVersion

// GroupV3 envia uma mensagem v3/FB a um grupo. Era Client.sendGroupV3.
func GroupV3(
	ctx context.Context,
	t Transport,
	to,
	ownID types.JID,
	id types.MessageID,
	messageApp []byte,
	msgAttrs msgattrs.MessageAttrs,
	frankingTag []byte,
	timings *DebugTimings,
) (string, []byte, error) {
	var groupMeta *group.Meta
	var err error
	start := time.Now()
	if to.Server == types.GroupServer {
		groupMeta, err = t.CachedGroupData(ctx, to)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get group members: %w", err)
		}
	}
	timings.GetParticipants = time.Since(start)
	// groupMeta stays nil both when `to` is not a group JID and when
	// CachedGroupData returns (nil, nil) — which it does if the server
	// echoed a different group `id` than the one queried, so the cache entry
	// landed under another key. Reading groupMeta.Members below would then be a
	// server-triggerable nil dereference.
	if groupMeta == nil {
		return "", nil, fmt.Errorf("failed to get group members: %w", group.ErrNotFound)
	}

	start = time.Now()
	builder := groups.NewGroupSessionBuilder(t.Store(), store.SignalProtobufSerializer)
	senderKeyName := protocol.NewSenderKeyName(to.String(), ownID.SignalAddress())
	signalSKDMessage, err := builder.Create(ctx, senderKeyName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create sender key distribution message to send %s to %s: %w", id, to, err)
	}
	skdm := &waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage{
		GroupID:                             proto.String(to.String()),
		AxolotlSenderKeyDistributionMessage: signalSKDMessage.Serialize(),
	}

	cipher := groups.NewGroupCipher(builder, senderKeyName, t.Store())
	plaintext, err := proto.Marshal(&waMsgTransport.MessageTransport{
		Payload: &waMsgTransport.MessageTransport_Payload{
			ApplicationPayload: &waCommon.SubProtocol{
				Payload: messageApp,
				Version: proto.Int32(FBApplicationVersion),
			},
			FutureProof: waCommon.FutureProofBehavior_PLACEHOLDER.Enum(),
		},
		Protocol: &waMsgTransport.MessageTransport_Protocol{
			Integral: &waMsgTransport.MessageTransport_Protocol_Integral{
				Padding: msgpad.Pad(nil),
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

	node, allDevices, err := PrepareMessageNodeV3(
		ctx, t, to, ownID, id, nil, skdm, msgAttrs, frankingTag, groupMeta.Members, timings,
	)
	if err != nil {
		return "", nil, err
	}

	phash := ParticipantListHashV2(allDevices)
	node.Attrs[msgAttrPHash] = phash
	skMsg := waBinary.Node{
		Tag:     encNodeTag,
		Content: ciphertext,
		Attrs: waBinary.Attrs{
			encAttrVersion: encVersionFB,
			encAttrType:    encTypeSenderKey,
		},
	}
	if msgAttrs.MediaType != "" {
		skMsg.Attrs[encAttrMediaType] = msgAttrs.MediaType
	}
	node.Content = append(node.GetChildren(), skMsg)

	start = time.Now()
	data, err := t.SendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return "", nil, fmt.Errorf("failed to send message node: %w", err)
	}
	return phash, data, nil
}

// DMV3 envia uma mensagem v3/FB a um usuario. Era Client.sendDMV3.
func DMV3(
	ctx context.Context,
	t Transport,
	to,
	ownID types.JID,
	id types.MessageID,
	messageApp []byte,
	msgAttrs msgattrs.MessageAttrs,
	frankingTag []byte,
	timings *DebugTimings,
) ([]byte, string, error) {
	payload := &waMsgTransport.MessageTransport_Payload{
		ApplicationPayload: &waCommon.SubProtocol{
			Payload: messageApp,
			Version: proto.Int32(FBApplicationVersion),
		},
		FutureProof: waCommon.FutureProofBehavior_PLACEHOLDER.Enum(),
	}

	node, allDevices, err := PrepareMessageNodeV3(ctx, t, to, ownID, id, payload, nil, msgAttrs, frankingTag, []types.JID{to, ownID.ToNonAD()}, timings)
	if err != nil {
		return nil, "", err
	}
	start := time.Now()
	data, err := t.SendNodeAndGetData(ctx, *node)
	timings.Send = time.Since(start)
	if err != nil {
		return nil, "", fmt.Errorf("failed to send message node: %w", err)
	}
	return data, ParticipantListHashV2(allDevices), nil
}

// PrepareMessageNodeV3 monta o <message> v3/FB. Era
// Client.prepareMessageNodeV3.
func PrepareMessageNodeV3(
	ctx context.Context,
	t Transport,
	to,
	ownID types.JID,
	id types.MessageID,
	payload *waMsgTransport.MessageTransport_Payload,
	skdm *waMsgTransport.MessageTransport_Protocol_Ancillary_SenderKeyDistributionMessage,
	msgAttrs msgattrs.MessageAttrs,
	frankingTag []byte,
	participants []types.JID,
	timings *DebugTimings,
) (*waBinary.Node, []types.JID, error) {
	start := time.Now()
	allDevices, err := t.UserDevices(ctx, participants)
	timings.GetDevices = time.Since(start)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get device list: %w", err)
	}

	encAttrs := waBinary.Attrs{}
	attrs := waBinary.Attrs{
		msgAttrID:   id,
		msgAttrType: msgAttrs.Type,
		msgAttrTo:   to,
	}
	// Only include mediatype on DMs, for groups it's in the skmsg node
	if payload != nil && msgAttrs.MediaType != "" {
		encAttrs[encAttrMediaType] = msgAttrs.MediaType
	}
	if msgAttrs.Edit != "" {
		attrs[msgAttrEdit] = string(msgAttrs.Edit)
	}
	if msgAttrs.DecryptFail != "" {
		encAttrs[encAttrDecryptFail] = string(msgAttrs.DecryptFail)
	}

	dsm := &waMsgTransport.MessageTransport_Protocol_Integral_DeviceSentMessage{
		DestinationJID: proto.String(to.String()),
		Phash:          proto.String(""),
	}

	start = time.Now()
	participantNodes, err := EncryptForDevicesV3(ctx, t, allDevices, ownID, id, payload, skdm, dsm, encAttrs)
	if err != nil {
		return nil, nil, err
	}
	timings.PeerEncrypt = time.Since(start)
	content := make([]waBinary.Node, 0, 4)
	content = append(content, waBinary.Node{
		Tag:     participantsNodeTag,
		Content: participantNodes,
	})
	metaAttrs := make(waBinary.Attrs)
	if msgAttrs.PollType != "" {
		metaAttrs[metaAttrPollType] = msgAttrs.PollType
	}
	if msgAttrs.DecryptFail != "" {
		metaAttrs[metaAttrDecryptFail] = string(msgAttrs.DecryptFail)
	}
	if len(metaAttrs) > 0 {
		content = append(content, waBinary.Node{
			Tag:   metaNodeTag,
			Attrs: metaAttrs,
		})
	}
	traceRequestID := uuid.New()
	content = append(content, waBinary.Node{
		Tag: frankingNodeTag,
		Content: []waBinary.Node{{
			Tag:     frankingTagNodeTag,
			Content: frankingTag,
		}},
	}, waBinary.Node{
		Tag: traceNodeTag,
		Content: []waBinary.Node{{
			Tag:     traceRequestIDNodeTag,
			Content: traceRequestID[:],
		}},
	})
	return &waBinary.Node{
		Tag:     messageNodeTag,
		Attrs:   attrs,
		Content: content,
	}, allDevices, nil
}
