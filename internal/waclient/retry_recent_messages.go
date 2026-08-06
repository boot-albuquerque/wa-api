// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/proto/waMsgApplication"
	"wa-api/internal/waclient/types"
	"wa-api/internal/waclient/types/events"
)

// Number of sent messages to cache in memory for handling retry receipts.
const recentMessagesSize = 256

type recentMessageKey struct {
	To types.JID
	ID types.MessageID
}

type RecentMessage struct {
	wa *waE2E.Message
	fb *waMsgApplication.MessageApplication
}

func (rm RecentMessage) IsEmpty() bool {
	return rm.wa == nil && rm.fb == nil
}

func (cli *Client) addRecentMessage(ctx context.Context, to types.JID, id types.MessageID, wa *waE2E.Message, fb *waMsgApplication.MessageApplication) error {
	if cli.UseRetryMessageStore {
		var buf []byte
		var format string
		var err error
		if wa != nil {
			buf, err = proto.Marshal(wa)
			format = "wa"
		} else if fb != nil {
			buf, err = proto.Marshal(fb)
			format = "fb"
		}
		if err != nil {
			return fmt.Errorf("failed to marshal message for retry store: %w", err)
		}
		if buf != nil {
			err = cli.Store.EventBuffer.AddOutgoingEvent(ctx, to, id, format, buf)
			if err != nil {
				return fmt.Errorf("failed to add message to retry store: %w", err)
			}
			if time.Since(cli.lastRetryStoreClear) > 12*time.Hour {
				err = cli.Store.EventBuffer.DeleteOldOutgoingEvents(ctx)
				if err != nil {
					return fmt.Errorf("failed to clear old messages from retry store: %w", err)
				}
			}
		}
	}
	cli.recentMessagesLock.Lock()
	key := recentMessageKey{to, id}
	if cli.recentMessagesList[cli.recentMessagesPtr].ID != "" {
		delete(cli.recentMessagesMap, cli.recentMessagesList[cli.recentMessagesPtr])
	}
	cli.recentMessagesMap[key] = RecentMessage{wa: wa, fb: fb}
	cli.recentMessagesList[cli.recentMessagesPtr] = key
	cli.recentMessagesPtr++
	if cli.recentMessagesPtr >= len(cli.recentMessagesList) {
		cli.recentMessagesPtr = 0
	}
	cli.recentMessagesLock.Unlock()
	return nil
}

func (cli *Client) getRecentMessage(to types.JID, id types.MessageID) RecentMessage {
	cli.recentMessagesLock.RLock()
	defer cli.recentMessagesLock.RUnlock()
	return cli.recentMessagesMap[recentMessageKey{to, id}]
}

func (cli *Client) getMessageForRetry(ctx context.Context, receipt *events.Receipt, messageID types.MessageID) (*RecentMessage, error) {
	msg := cli.getRecentMessage(receipt.Chat, messageID)
	if !msg.IsEmpty() {
		cli.Log.Debugf("Found message in local cache to accept retry receipt for %s/%s from %s", receipt.Chat, messageID, receipt.Sender)
		return &msg, nil
	}
	var altChat types.JID
	var err error
	switch receipt.Chat.Server {
	case types.DefaultUserServer:
		altChat, err = cli.Store.LIDs.GetLIDForPN(ctx, receipt.Chat)
	case types.HiddenUserServer:
		altChat, err = cli.Store.LIDs.GetPNForLID(ctx, receipt.Chat)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alternate JID for %s: %w", receipt.Chat, err)
	} else if !altChat.IsEmpty() {
		msg = cli.getRecentMessage(altChat, messageID)
		if !msg.IsEmpty() {
			cli.Log.Debugf("Found message in local cache with alternate chat JID %s to accept retry receipt for %s/%s from %s", altChat, receipt.Chat, messageID, receipt.Sender)
			return &msg, nil
		}
	}
	if cli.UseRetryMessageStore {
		format, buf, err := cli.Store.EventBuffer.GetOutgoingEvent(ctx, receipt.Chat, altChat, messageID)
		if err != nil {
			return nil, fmt.Errorf("failed to get message from retry store: %w", err)
		}
		return parseRecentMessage(format, buf)
	}
	waMsg := cli.GetMessageForRetry(receipt.Sender, receipt.Chat, messageID)
	if waMsg != nil {
		cli.Log.Debugf("Found message in GetMessageForRetry to accept retry receipt for %s/%s from %s", receipt.Chat, messageID, receipt.Sender)
		return &RecentMessage{wa: waMsg}, nil
	}
	return nil, nil
}

func parseRecentMessage(format string, buf []byte) (*RecentMessage, error) {
	var rm RecentMessage
	var err error
	switch format {
	case "wa":
		rm.wa = &waE2E.Message{}
		err = proto.Unmarshal(buf, rm.wa)
	case "fb":
		rm.fb = &waMsgApplication.MessageApplication{}
		err = proto.Unmarshal(buf, rm.fb)
	default:
		err = fmt.Errorf("unknown format in retry store: %s", format)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload in retry store: %w", err)
	}
	return &rm, nil
}
