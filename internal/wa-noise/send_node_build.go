// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"slices"
	"time"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

func (cli *Client) preparePeerMessageNode(
	ctx context.Context,
	to types.JID,
	id types.MessageID,
	message *waE2E.Message,
	timings *MessageDebugTimings,
) (*waBinary.Node, error) {
	attrs := waBinary.Attrs{
		"id":       id,
		"type":     "text",
		"category": "peer",
		"to":       to,
	}
	if message.GetProtocolMessage().GetType() == waE2E.ProtocolMessage_APP_STATE_SYNC_KEY_REQUEST {
		attrs["push_priority"] = "high"
	} else if message.GetProtocolMessage().GetPeerDataOperationRequestMessage().GetPeerDataOperationRequestType() == waE2E.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND {
		attrs["push_priority"] = "high_force"
		attrs["privacy_sensitive"] = "1"
	}
	start := time.Now()
	plaintext, err := proto.Marshal(message)
	timings.Marshal = time.Since(start)
	if err != nil {
		err = fmt.Errorf("failed to marshal message: %w", err)
		return nil, err
	}
	encryptionIdentity := to
	if to.Server == types.DefaultUserServer {
		encryptionIdentity, err = cli.Store.LIDs.GetLIDForPN(ctx, to)
		if err != nil {
			return nil, fmt.Errorf("failed to get LID for PN %s: %w", to, err)
		}
	}
	start = time.Now()
	encrypted, isPreKey, err := cli.encryptMessageForDevice(ctx, plaintext, encryptionIdentity, nil, nil, nil)
	timings.PeerEncrypt = time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt peer message for %s: %v", to, err)
	}
	content := []waBinary.Node{{
		Tag: "meta",
		Attrs: waBinary.Attrs{
			"appdata": "default",
		},
	}, *encrypted}
	if isPreKey && cli.MessengerConfig == nil {
		content = append(content, cli.makeDeviceIdentityNode())
	}
	return &waBinary.Node{
		Tag:     "message",
		Attrs:   attrs,
		Content: content,
	}, nil
}

func (cli *Client) getMessageContent(
	baseNode waBinary.Node,
	message *waE2E.Message,
	msgAttrs waBinary.Attrs,
	includeIdentity bool,
	extraParams nodeExtraParams,
) []waBinary.Node {
	content := []waBinary.Node{baseNode}
	if includeIdentity {
		content = append(content, cli.makeDeviceIdentityNode())
	}
	if msgAttrs["type"] == "poll" {
		pollType := "creation"
		if message.PollUpdateMessage != nil {
			pollType = "vote"
		}
		content = append(content, waBinary.Node{
			Tag: "meta",
			Attrs: waBinary.Attrs{
				"polltype": pollType,
			},
		})
	}

	if extraParams.botNode != nil {
		content = append(content, *extraParams.botNode)
	}
	if extraParams.metaNode != nil {
		content = append(content, *extraParams.metaNode)
	}
	if extraParams.additionalNodes != nil {
		content = append(content, *extraParams.additionalNodes...)
	}

	if buttonType := getButtonTypeFromMessage(message); buttonType != "" {
		content = append(content, waBinary.Node{
			Tag: "biz",
			Content: []waBinary.Node{{
				Tag:   buttonType,
				Attrs: getButtonAttributes(message),
			}},
		})
	}
	return content
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
	start := time.Now()
	allDevices, err := cli.GetUserDevices(ctx, participants)
	timings.GetDevices = time.Since(start)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get device list: %w", err)
	}

	if to.Server == types.GroupServer {
		allDevices = slices.DeleteFunc(allDevices, func(jid types.JID) bool {
			return jid.Server == types.HostedServer || jid.Server == types.HostedLIDServer
		})
	}

	msgType := getTypeFromMessage(message)
	encAttrs := waBinary.Attrs{}
	// Only include encMediaType for 1:1 messages (groups don't have a device-sent message plaintext)
	if encMediaType := getMediaTypeFromMessage(message); dsmPlaintext != nil && encMediaType != "" {
		encAttrs["mediatype"] = encMediaType
	}
	attrs := waBinary.Attrs{
		"id":   id,
		"type": msgType,
		"to":   to,
	}
	// TODO this is a very hacky hack for announcement group messages, why is it pn anyway?
	if extraParams.addressingMode != "" {
		attrs["addressing_mode"] = string(extraParams.addressingMode)
	}
	if editAttr := getEditAttribute(message); editAttr != "" {
		attrs["edit"] = string(editAttr)
		encAttrs["decrypt-fail"] = string(events.DecryptFailHide)
	}
	if msgType == "reaction" || message.GetPollUpdateMessage() != nil {
		encAttrs["decrypt-fail"] = string(events.DecryptFailHide)
	}

	start = time.Now()
	participantNodes, includeIdentity, err := cli.encryptMessageForDevices(
		ctx, allDevices, id, plaintext, dsmPlaintext, encAttrs,
	)
	timings.PeerEncrypt = time.Since(start)
	if err != nil {
		return nil, nil, err
	}
	participantNode := waBinary.Node{
		Tag:     "participants",
		Content: participantNodes,
	}
	return &waBinary.Node{
		Tag:   "message",
		Attrs: attrs,
		Content: cli.getMessageContent(
			participantNode, message, attrs, includeIdentity, extraParams,
		),
	}, allDevices, nil
}

func marshalMessage(to types.JID, message *waE2E.Message) (plaintext, dsmPlaintext []byte, err error) {
	if message == nil && to.Server == types.NewsletterServer {
		return
	}
	plaintext, err = proto.Marshal(message)
	if err != nil {
		err = fmt.Errorf("failed to marshal message: %w", err)
		return
	}

	if to.Server != types.GroupServer && to.Server != types.NewsletterServer {
		dsmPlaintext, err = proto.Marshal(&waE2E.Message{
			DeviceSentMessage: &waE2E.DeviceSentMessage{
				DestinationJID: proto.String(to.String()),
				Message:        message,
			},
			MessageContextInfo: message.MessageContextInfo,
		})
		if err != nil {
			err = fmt.Errorf("failed to marshal message (for own devices): %w", err)
			return
		}
	}

	return
}

func (cli *Client) makeDeviceIdentityNode() waBinary.Node {
	deviceIdentity, err := proto.Marshal(cli.Store.Account)
	if err != nil {
		panic(fmt.Errorf("failed to marshal device identity: %w", err))
	}
	return waBinary.Node{
		Tag:     "device-identity",
		Content: deviceIdentity,
	}
}
