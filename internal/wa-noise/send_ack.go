// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// Attribute names of the server ack node of an outgoing message.
const (
	ackAttrServerID = "server_id"
	ackAttrTime     = "t"
	ackAttrError    = "error"
	ackAttrPHash    = "phash"
)

// retryFrameContext is the human-readable context passed to retryFrame when
// the server answers a message send with a disconnect node.
const retryFrameContext = "message send"

// awaitSendAck waits for the server ack of an outgoing message, honouring the
// request timeout and the caller's context, and transparently retries the
// frame if the server answered with a disconnect node. `start` is the instant
// the wait began, used to fill resp.DebugTimings.Resp.
func (cli *Client) awaitSendAck(
	ctx context.Context,
	req *SendRequestExtra,
	resp *SendResponse,
	respChan chan *waBinary.Node,
	data []byte,
	start time.Time,
) (*waBinary.Node, error) {
	var timeoutChan <-chan time.Time
	if req.Timeout > 0 {
		timeoutChan = time.After(req.Timeout)
	} else {
		timeoutChan = make(<-chan time.Time)
	}
	var respNode *waBinary.Node
	select {
	case respNode = <-respChan:
	case <-timeoutChan:
		cli.cancelResponse(req.ID, respChan)
		return nil, ErrMessageTimedOut
	case <-ctx.Done():
		cli.cancelResponse(req.ID, respChan)
		return nil, ctx.Err()
	}
	resp.DebugTimings.Resp = time.Since(start)
	if isDisconnectNode(respNode) {
		retryStart := time.Now()
		var err error
		respNode, err = cli.retryFrame(ctx, retryFrameContext, req.ID, data, respNode, 0)
		resp.DebugTimings.Retry = time.Since(retryStart)
		if err != nil {
			return nil, err
		}
	}
	return respNode, nil
}

// applySendAck copies the server ack attributes into resp and invalidates the
// group/device caches when the server reports a different participant list
// hash than the one the message was encrypted for.
func (cli *Client) applySendAck(respNode *waBinary.Node, to types.JID, phash string, resp *SendResponse) error {
	ag := respNode.AttrGetter()
	resp.ServerID = types.MessageServerID(ag.OptionalInt(ackAttrServerID))
	resp.Timestamp = ag.UnixTime(ackAttrTime)
	var err error
	if errorCode := ag.Int(ackAttrError); errorCode != 0 {
		err = fmt.Errorf("%w %d", ErrServerReturnedError, errorCode)
	}
	expectedPHash := ag.OptionalString(ackAttrPHash)
	if len(expectedPHash) > 0 && phash != expectedPHash {
		cli.Log.Warnf("Server returned different participant list hash (%s != %s) when sending to %s. Some devices may not have received the message.", phash, expectedPHash, to)
		cli.invalidateParticipantCache(to)
	}
	return err
}

// invalidateParticipantCache drops the cached participant/device list of the
// given destination so the next send re-fetches it from the server.
func (cli *Client) invalidateParticipantCache(to types.JID) {
	switch to.Server {
	case types.GroupServer:
		// TODO also invalidate device list caches
		cli.groupCacheLock.Lock()
		delete(cli.groupCache, to)
		cli.groupCacheLock.Unlock()
	case types.BroadcastServer:
		// TODO do something
	case types.DefaultUserServer, types.HiddenUserServer, types.BotServer, types.HostedServer, types.HostedLIDServer:
		cli.userDevicesCacheLock.Lock()
		delete(cli.userDevicesCache, to)
		cli.userDevicesCacheLock.Unlock()
	}
}
