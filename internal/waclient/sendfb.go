// Copyright (c) 2024 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/util/random"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/waclient/binary"
	armadillo "wa-api/internal/waclient/proto"
	"wa-api/internal/waclient/proto/waArmadilloApplication"
	"wa-api/internal/waclient/proto/waCommon"
	"wa-api/internal/waclient/proto/waConsumerApplication"
	"wa-api/internal/waclient/proto/waMsgApplication"
	"wa-api/internal/waclient/types"
)

const FBMessageVersion = 3
const FBMessageApplicationVersion = 2
const IGMessageApplicationVersion = 3
const FBConsumerMessageVersion = 1
const FBArmadilloMessageVersion = 1

// SendFBMessage sends the given v3 message to the given JID.
func (cli *Client) SendFBMessage(
	ctx context.Context,
	to types.JID,
	message armadillo.RealMessageApplicationSub,
	metadata *waMsgApplication.MessageApplication_Metadata,
	extra ...SendRequestExtra,
) (resp SendResponse, err error) {
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
	var subproto waMsgApplication.MessageApplication_SubProtocolPayload
	subproto.FutureProof = waCommon.FutureProofBehavior_PLACEHOLDER.Enum()
	switch typedMsg := message.(type) {
	case *waConsumerApplication.ConsumerApplication:
		var consumerMessage []byte
		consumerMessage, err = proto.Marshal(typedMsg)
		if err != nil {
			err = fmt.Errorf("failed to marshal consumer message: %w", err)
			return
		}
		subproto.SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_ConsumerMessage{
			ConsumerMessage: &waCommon.SubProtocol{
				Payload: consumerMessage,
				Version: proto.Int32(FBConsumerMessageVersion),
			},
		}
	case *waArmadilloApplication.Armadillo:
		var armadilloMessage []byte
		armadilloMessage, err = proto.Marshal(typedMsg)
		if err != nil {
			err = fmt.Errorf("failed to marshal armadillo message: %w", err)
			return
		}
		subproto.SubProtocol = &waMsgApplication.MessageApplication_SubProtocolPayload_Armadillo{
			Armadillo: &waCommon.SubProtocol{
				Payload: armadilloMessage,
				Version: proto.Int32(FBArmadilloMessageVersion),
			},
		}
	default:
		err = fmt.Errorf("unsupported message type %T", message)
		return
	}
	if metadata == nil {
		metadata = &waMsgApplication.MessageApplication_Metadata{}
	}
	metadata.FrankingVersion = proto.Int32(0)
	metadata.FrankingKey = random.Bytes(32)
	msgAttrs := getAttrsFromFBMessage(message)
	messageAppProto := &waMsgApplication.MessageApplication{
		Payload: &waMsgApplication.MessageApplication_Payload{
			Content: &waMsgApplication.MessageApplication_Payload_SubProtocol{
				SubProtocol: &subproto,
			},
		},
		Metadata: metadata,
	}
	messageApp, err := proto.Marshal(messageAppProto)
	if err != nil {
		return resp, fmt.Errorf("failed to marshal message application: %w", err)
	}
	frankingHash := hmac.New(sha256.New, metadata.FrankingKey)
	frankingHash.Write(messageApp)
	frankingTag := frankingHash.Sum(nil)
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
	resp.ID = req.ID

	start := time.Now()
	// Sending multiple messages at a time can cause weird issues and makes it harder to retry safely
	cli.messageSendLock.Lock()
	resp.DebugTimings.Queue = time.Since(start)
	defer cli.messageSendLock.Unlock()

	if !req.Peer {
		err = cli.addRecentMessage(ctx, to, req.ID, nil, messageAppProto)
		if err != nil {
			return
		}
	}
	respChan := cli.waitResponse(req.ID)
	var phash string
	var data []byte
	switch to.Server {
	case types.GroupServer:
		phash, data, err = cli.sendGroupV3(ctx, to, ownID, req.ID, messageApp, msgAttrs, frankingTag, &resp.DebugTimings)
	case types.DefaultUserServer, types.MessengerServer:
		if req.Peer {
			err = fmt.Errorf("peer messages to fb are not yet supported")
			//data, err = cli.sendPeerMessage(to, req.ID, message, &resp.DebugTimings)
		} else {
			data, phash, err = cli.sendDMV3(ctx, to, ownID, req.ID, messageApp, msgAttrs, frankingTag, &resp.DebugTimings)
		}
	default:
		err = fmt.Errorf("%w %s", ErrUnknownServer, to.Server)
	}
	start = time.Now()
	if err != nil {
		cli.cancelResponse(req.ID, respChan)
		return
	}
	var respNode *waBinary.Node
	var timeoutChan <-chan time.Time
	if req.Timeout > 0 {
		timeoutChan = time.After(req.Timeout)
	} else {
		timeoutChan = make(<-chan time.Time)
	}
	select {
	case respNode = <-respChan:
	case <-timeoutChan:
		cli.cancelResponse(req.ID, respChan)
		err = ErrMessageTimedOut
		return
	case <-ctx.Done():
		cli.cancelResponse(req.ID, respChan)
		err = ctx.Err()
		return
	}
	resp.DebugTimings.Resp = time.Since(start)
	if isDisconnectNode(respNode) {
		start = time.Now()
		respNode, err = cli.retryFrame(ctx, "message send", req.ID, data, respNode, 0)
		resp.DebugTimings.Retry = time.Since(start)
		if err != nil {
			return
		}
	}
	ag := respNode.AttrGetter()
	resp.ServerID = types.MessageServerID(ag.OptionalInt("server_id"))
	resp.Timestamp = ag.UnixTime("t")
	if errorCode := ag.Int("error"); errorCode != 0 {
		err = fmt.Errorf("%w %d", ErrServerReturnedError, errorCode)
	}
	expectedPHash := ag.OptionalString("phash")
	if len(expectedPHash) > 0 && phash != expectedPHash {
		cli.Log.Warnf("Server returned different participant list hash when sending to %s. Some devices may not have received the message.", to)
		// TODO also invalidate device list caches
		cli.groupCacheLock.Lock()
		delete(cli.groupCache, to)
		cli.groupCacheLock.Unlock()
	}
	return
}
