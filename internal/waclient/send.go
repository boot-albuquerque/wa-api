// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"fmt"
	"time"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/types"
)

// SendMessage sends the given message.
//
// This method will wait for the server to acknowledge the message before returning.
// The return value is the timestamp of the message from the server.
//
// Optional parameters like the message ID can be specified with the SendRequestExtra struct.
// Only one extra parameter is allowed, put all necessary parameters in the same struct.
//
// The message itself can contain anything you want (within the protobuf schema).
// e.g. for a simple text message, use the Conversation field:
//
//	cli.SendMessage(context.Background(), targetJID, &waE2E.Message{
//		Conversation: proto.String("Hello, World!"),
//	})
//
// Things like replies, mentioning users and the "forwarded" flag are stored in ContextInfo,
// which can be put in ExtendedTextMessage and any of the media message types.
//
// For uploading and sending media/attachments, see the Upload method.
//
// For other message types, you'll have to figure it out yourself. Looking at the protobuf schema
// in binary/proto/def.proto may be useful to find out all the allowed fields. Printing the RawMessage
// field in incoming message events to figure out what it contains is also a good way to learn how to
// send the same kind of message.
func (cli *Client) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...SendRequestExtra) (resp SendResponse, err error) {
	if cli == nil {
		err = ErrClientIsNil
		return
	}
	var req SendRequestExtra
	if len(extra) > 1 {
		err = errors.New("only one extra parameter may be provided to SendMessage")
		return
	} else if len(extra) == 1 {
		req = extra[0]
	}
	if to.Device > 0 && !req.Peer {
		err = ErrRecipientADJID
		return
	}
	ownID := cli.getOwnID()
	if ownID.IsEmpty() {
		err = ErrNotLoggedIn
		return
	}

	if req.Timeout == 0 {
		req.Timeout = defaultRequestTimeout
	}
	if len(req.ID) == 0 {
		req.ID = cli.GenerateMessageID()
	}
	if to.Server == types.NewsletterServer {
		// TODO somehow deduplicate this with the code in sendNewsletter?
		if message.EditedMessage != nil {
			req.ID = types.MessageID(message.GetEditedMessage().GetMessage().GetProtocolMessage().GetKey().GetID())
		} else if message.ProtocolMessage != nil && message.ProtocolMessage.GetType() == waE2E.ProtocolMessage_REVOKE {
			req.ID = types.MessageID(message.GetProtocolMessage().GetKey().GetID())
		}
	}
	resp.ID = req.ID

	var extraParams nodeExtraParams
	message, err = cli.prepareBotMessage(ctx, &req, to, message, resp.ID, &extraParams)
	if err != nil {
		return
	}

	var groupParticipants []types.JID
	groupParticipants, err = cli.resolveSendTarget(ctx, &to, &ownID, &req, &resp, &extraParams)
	if err != nil {
		return
	}
	applyRequestExtraNodes(&req, &extraParams)

	resp.Sender = ownID

	start := time.Now()
	// Sending multiple messages at a time can cause weird issues and makes it harder to retry safely
	// This is also required for the session prefetching that makes group sends faster
	// (everything will explode if you send a message to the same user twice in parallel)
	cli.messageSendLock.Lock()
	resp.DebugTimings.Queue = time.Since(start)
	defer cli.messageSendLock.Unlock()

	// Peer message retries aren't implemented yet
	if !req.Peer {
		err = cli.addRecentMessage(ctx, to, req.ID, message, nil)
		if err != nil {
			return
		}
	}

	if message.GetMessageContextInfo().GetMessageSecret() != nil {
		err = cli.Store.MsgSecrets.PutMessageSecret(ctx, to, ownID, req.ID, message.GetMessageContextInfo().GetMessageSecret())
		if err != nil {
			cli.Log.Warnf("Failed to store message secret key for outgoing message %s: %v", req.ID, err)
		} else {
			cli.Log.Debugf("Stored message secret key for outgoing message %s", req.ID)
		}
	}

	respChan := cli.waitResponse(req.ID)
	var phash string
	var data []byte
	switch to.Server {
	case types.GroupServer, types.BroadcastServer:
		phash, data, err = cli.sendGroup(ctx, ownID, to, groupParticipants, req.ID, message, &resp.DebugTimings, extraParams)
	case types.DefaultUserServer, types.BotServer, types.HiddenUserServer:
		if req.Peer {
			data, err = cli.sendPeerMessage(ctx, to, req.ID, message, &resp.DebugTimings)
		} else {
			phash, data, err = cli.sendDM(ctx, ownID, to, req.ID, message, &resp.DebugTimings, extraParams)
		}
	case types.NewsletterServer:
		data, err = cli.sendNewsletter(ctx, to, req.ID, message, req.MediaHandle, &resp.DebugTimings)
	default:
		err = fmt.Errorf("%w %s", ErrUnknownServer, to.Server)
	}
	start = time.Now()
	if err != nil {
		cli.cancelResponse(req.ID, respChan)
		return
	}
	var respNode *waBinary.Node
	respNode, err = cli.awaitSendAck(ctx, &req, &resp, respChan, data, start)
	if err != nil {
		return
	}
	err = cli.applySendAck(respNode, to, phash, &resp)
	return
}

func (cli *Client) SendPeerMessage(ctx context.Context, message *waE2E.Message) (SendResponse, error) {
	ownID := cli.getOwnID().ToNonAD()
	if ownID.IsEmpty() {
		return SendResponse{}, ErrNotLoggedIn
	}
	return cli.SendMessage(ctx, ownID, message, SendRequestExtra{Peer: true})
}

// RevokeMessage deletes the given message from everyone in the chat.
//
// This method will wait for the server to acknowledge the revocation message before returning.
// The return value is the timestamp of the message from the server.
//
// Deprecated: This method is deprecated in favor of BuildRevoke
func (cli *Client) RevokeMessage(ctx context.Context, chat types.JID, id types.MessageID) (SendResponse, error) {
	return cli.SendMessage(ctx, chat, cli.BuildRevoke(chat, types.EmptyJID, id))
}
