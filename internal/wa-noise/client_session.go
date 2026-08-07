// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package whatsmeow implements a client for interacting with the WhatsApp web multidevice API.
package whatsmeow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waWeb"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Logout sends a request to unlink the device, then disconnects from the websocket and deletes the local device store.
//
// If the logout request fails, the disconnection and local data deletion will not happen either.
// If an error is returned, but you want to force disconnect/clear data, call Client.Disconnect() and Client.Store.Delete() manually.
//
// Note that this will not emit any events. The LoggedOut event is only used for external logouts
// (triggered by the user from the main device or by WhatsApp servers).
func (cli *Client) Logout(ctx context.Context) error {
	if cli == nil {
		return ErrClientIsNil
	} else if cli.MessengerConfig != nil {
		return errors.New("can't logout with Messenger credentials")
	}
	ownID := cli.getOwnID()
	if ownID.IsEmpty() {
		return ErrNotLoggedIn
	}
	_, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "md",
		Type:      "set",
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "remove-companion-device",
			Attrs: waBinary.Attrs{
				"jid":    ownID,
				"reason": "user_initiated",
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("error sending logout request: %w", err)
	}
	cli.Disconnect()
	err = cli.Store.Delete(ctx)
	if err != nil {
		return fmt.Errorf("error deleting data from store: %w", err)
	}
	return nil
}

// ParseWebMessage parses a WebMessageInfo object into *events.Message to match what real-time messages have.
//
// The chat JID can be found in the Conversation data:
//
//	chatJID, err := types.ParseJID(conv.GetId())
//	for _, historyMsg := range conv.GetMessages() {
//		evt, err := cli.ParseWebMessage(chatJID, historyMsg.GetMessage())
//		yourNormalEventHandler(evt)
//	}
func (cli *Client) ParseWebMessage(chatJID types.JID, webMsg *waWeb.WebMessageInfo) (*events.Message, error) {
	var err error
	if chatJID.IsEmpty() {
		chatJID, err = types.ParseJID(webMsg.GetKey().GetRemoteJID())
		if err != nil {
			return nil, fmt.Errorf("no chat JID provided and failed to parse remote JID: %w", err)
		}
	}
	info := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chatJID,
			IsFromMe: webMsg.GetKey().GetFromMe(),
			IsGroup:  chatJID.Server == types.GroupServer,
		},
		ID:        webMsg.GetKey().GetID(),
		PushName:  webMsg.GetPushName(),
		Timestamp: time.Unix(int64(webMsg.GetMessageTimestamp()), 0),
	}
	if info.IsFromMe {
		info.Sender = cli.getOwnID().ToNonAD()
		if info.Sender.IsEmpty() {
			return nil, ErrNotLoggedIn
		}
	} else if chatJID.Server == types.DefaultUserServer || chatJID.Server == types.HiddenUserServer || chatJID.Server == types.NewsletterServer {
		info.Sender = chatJID
	} else if webMsg.GetParticipant() != "" {
		info.Sender, err = types.ParseJID(webMsg.GetParticipant())
	} else if webMsg.GetKey().GetParticipant() != "" {
		info.Sender, err = types.ParseJID(webMsg.GetKey().GetParticipant())
	} else {
		return nil, fmt.Errorf("couldn't find sender of message %s", info.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse sender of message %s: %v", info.ID, err)
	}
	if pk := webMsg.GetCommentMetadata().GetCommentParentKey(); pk != nil {
		info.MsgMetaInfo.ThreadMessageID = pk.GetID()
		info.MsgMetaInfo.ThreadMessageSenderJID, _ = types.ParseJID(pk.GetParticipant())
	}
	evt := &events.Message{
		RawMessage:   webMsg.GetMessage(),
		SourceWebMsg: webMsg,
		Info:         info,
	}
	evt.UnwrapRaw()
	if evt.Message.GetProtocolMessage().GetType() == waE2E.ProtocolMessage_MESSAGE_EDIT {
		evt.Info.ID = evt.Message.GetProtocolMessage().GetKey().GetID()
		evt.Message = evt.Message.GetProtocolMessage().GetEditedMessage()
	}
	return evt, nil
}

func (cli *Client) StoreLIDPNMapping(ctx context.Context, first, second types.JID) {
	var lid, pn types.JID
	if first.Server == types.HiddenUserServer && second.Server == types.DefaultUserServer {
		lid = first
		pn = second
	} else if first.Server == types.DefaultUserServer && second.Server == types.HiddenUserServer {
		lid = second
		pn = first
	} else {
		return
	}
	err := cli.Store.LIDs.PutLIDMapping(ctx, lid, pn)
	if err != nil {
		cli.Log.Errorf("Failed to store LID-PN mapping for %s -> %s: %v", lid, pn, err)
	}
}

const unifiedOffset = 3 * 24 * time.Hour
const week = 7 * 24 * time.Hour

func (cli *Client) getUnifiedSessionID() string {
	unifiedTS := time.Now().
		Add(time.Duration(cli.serverTimeOffset.Load())).
		Add(unifiedOffset)
	unifiedID := unifiedTS.UnixMilli() % week.Milliseconds()
	return strconv.FormatInt(unifiedID, 10)
}

func (cli *Client) sendUnifiedSession() {
	if cli == nil {
		return
	}

	node := waBinary.Node{
		Tag:   "ib",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{{
			Tag: "unified_session",
			Attrs: waBinary.Attrs{
				"id": cli.getUnifiedSessionID(),
			},
		}},
	}

	err := cli.sendNode(cli.BackgroundEventCtx, node)
	if err != nil {
		cli.Log.Debugf("Failed to send unified_session telemetry: %v", err)
	}
}
