// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package send

import (
	"context"
	"fmt"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
)

// AwaitAck waits for the server ack of an outgoing message, honouring the
// request timeout and the caller's context, and transparently retries the
// frame if the server answered with a disconnect node. `start` is the instant
// the wait began, used to fill resp.DebugTimings.Resp.
//
// Era Client.awaitSendAck.
func AwaitAck(
	ctx context.Context,
	t Transport,
	req *RequestExtra,
	resp *Response,
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
		t.CancelResponse(req.ID, respChan)
		return nil, ErrMessageTimedOut
	case <-ctx.Done():
		t.CancelResponse(req.ID, respChan)
		return nil, ctx.Err()
	}
	resp.DebugTimings.Resp = time.Since(start)
	if t.IsDisconnectNode(respNode) {
		retryStart := time.Now()
		var err error
		respNode, err = t.RetryFrame(ctx, retryFrameContext, req.ID, data, respNode, 0)
		resp.DebugTimings.Retry = time.Since(retryStart)
		if err != nil {
			return nil, err
		}
	}
	return respNode, nil
}

// ApplyAck copies the server ack attributes into resp and invalidates the
// group/device caches when the server reports a different participant list
// hash than the one the message was encrypted for.
//
// Era Client.applySendAck.
func ApplyAck(t Transport, respNode *waBinary.Node, to types.JID, phash string, resp *Response) error {
	ag := respNode.AttrGetter()
	resp.ServerID = types.MessageServerID(ag.OptionalInt(ackAttrServerID))
	resp.Timestamp = ag.UnixTime(ackAttrTime)
	var err error
	if errorCode := ag.Int(ackAttrError); errorCode != 0 {
		err = fmt.Errorf("%w %d", ErrServerReturnedError, errorCode)
	}
	expectedPHash := ag.OptionalString(ackAttrPHash)
	if len(expectedPHash) > 0 && phash != expectedPHash {
		t.Log().Warnf("Server returned different participant list hash (%s != %s) when sending to %s. Some devices may not have received the message.", phash, expectedPHash, to)
		InvalidateParticipantCache(t, to)
	}
	return err
}

// InvalidateParticipantCache drops the cached participant/device list of the
// given destination so the next send re-fetches it from the server.
//
// Era Client.invalidateParticipantCache.
func InvalidateParticipantCache(t Transport, to types.JID) {
	switch to.Server {
	case types.GroupServer:
		// TODO also invalidate device list caches
		t.InvalidateGroupCache(to)
	case types.BroadcastServer:
		// TODO do something
	case types.DefaultUserServer, types.HiddenUserServer, types.BotServer, types.HostedServer, types.HostedLIDServer:
		t.InvalidateDeviceCache(to)
	}
}
