// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/binary"

	"go.mau.fi/libsignal/ecc"
	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

// sendRetryReceipt sends a retry receipt for an incoming message.
func (cli *Client) sendRetryReceipt(ctx context.Context, node *waBinary.Node, info *types.MessageInfo, forceIncludeIdentity bool) {
	id, _ := node.Attrs["id"].(string)
	children := node.GetChildren()
	var retryCountInMsg int
	if len(children) == 1 && children[0].Tag == "enc" {
		retryCountInMsg = children[0].AttrGetter().OptionalInt("count")
	}

	cli.messageRetriesLock.Lock()
	cli.messageRetries[id]++
	retryCount := cli.messageRetries[id]
	// In case the message is a retry response, and we restarted in between, find the count from the message
	if retryCount == 1 && retryCountInMsg > 0 {
		retryCount = retryCountInMsg + 1
		cli.messageRetries[id] = retryCount
	}
	cli.messageRetriesLock.Unlock()
	if retryCount >= 5 {
		cli.Log.Warnf("Not sending any more retry receipts for %s", id)
		return
	}
	if retryCount == 1 {
		if cli.SynchronousAck {
			cli.immediateRequestMessageFromPhone(ctx, info)
		} else {
			go cli.delayedRequestMessageFromPhone(info)
		}
	}

	var registrationIDBytes [4]byte
	binary.BigEndian.PutUint32(registrationIDBytes[:], cli.Store.RegistrationID)
	attrs := buildBaseReceipt(info.ID, node)
	attrs["type"] = "retry"
	if info.Type == "peer_msg" && info.IsFromMe {
		attrs["category"] = "peer"
	}
	payload := waBinary.Node{
		Tag:   "receipt",
		Attrs: attrs,
		Content: []waBinary.Node{
			{Tag: "retry", Attrs: waBinary.Attrs{
				"count": retryCount,
				"id":    id,
				"t":     node.Attrs["t"],
				"v":     1,
			}},
			{Tag: "registration", Content: registrationIDBytes[:]},
		},
	}
	if retryCount > 1 || forceIncludeIdentity {
		if key, err := cli.Store.PreKeys.GenOnePreKey(ctx); err != nil {
			cli.Log.Errorf("Failed to get prekey for retry receipt: %v", err)
		} else if deviceIdentity, err := proto.Marshal(cli.Store.Account); err != nil {
			cli.Log.Errorf("Failed to marshal account info: %v", err)
			return
		} else {
			payload.Content = append(payload.GetChildren(), waBinary.Node{
				Tag: "keys",
				Content: []waBinary.Node{
					{Tag: "type", Content: []byte{ecc.DjbType}},
					{Tag: "identity", Content: cli.Store.IdentityKey.Pub[:]},
					preKeyToNode(key),
					preKeyToNode(cli.Store.SignedPreKey),
					{Tag: "device-identity", Content: deviceIdentity},
				},
			})
		}
	}
	err := cli.sendNode(ctx, payload)
	if err != nil {
		cli.Log.Errorf("Failed to send retry receipt for %s: %v", id, err)
	}
}
