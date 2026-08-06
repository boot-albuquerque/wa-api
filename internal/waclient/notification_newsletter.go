// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/json"

	"google.golang.org/protobuf/proto"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/proto/waE2E"
	"wa-api/internal/waclient/types"
	"wa-api/internal/waclient/types/events"
)

func (cli *Client) parseNewsletterMessages(node *waBinary.Node) []*types.NewsletterMessage {
	children := node.GetChildren()
	output := make([]*types.NewsletterMessage, 0, len(children))
	for _, child := range children {
		if child.Tag != "message" {
			continue
		}
		ag := child.AttrGetter()
		msg := types.NewsletterMessage{
			MessageServerID: ag.Int("server_id"),
			MessageID:       ag.String("id"),
			Type:            ag.String("type"),
			Timestamp:       ag.UnixTime("t"),
			ViewsCount:      0,
			ReactionCounts:  nil,
		}
		for _, subchild := range child.GetChildren() {
			switch subchild.Tag {
			case "plaintext":
				byteContent, ok := subchild.Content.([]byte)
				if ok {
					msg.Message = new(waE2E.Message)
					err := proto.Unmarshal(byteContent, msg.Message)
					if err != nil {
						cli.Log.Warnf("Failed to unmarshal newsletter message: %v", err)
						msg.Message = nil
					}
				}
			case "views_count":
				msg.ViewsCount = subchild.AttrGetter().Int("count")
			case "reactions":
				msg.ReactionCounts = make(map[string]int)
				for _, reaction := range subchild.GetChildren() {
					rag := reaction.AttrGetter()
					msg.ReactionCounts[rag.String("code")] = rag.Int("count")
				}
			}
		}
		output = append(output, &msg)
	}
	return output
}

func (cli *Client) handleNewsletterNotification(ctx context.Context, node *waBinary.Node) {
	ag := node.AttrGetter()
	liveUpdates := node.GetChildByTag("live_updates")
	cli.dispatchEvent(&events.NewsletterLiveUpdate{
		JID:      ag.JID("from"),
		Time:     ag.UnixTime("t"),
		Messages: cli.parseNewsletterMessages(&liveUpdates),
	})
}

type newsLetterEventWrapper struct {
	Data newsletterEvent `json:"data"`
}

type newsletterEvent struct {
	Join       *events.NewsletterJoin       `json:"xwa2_notify_newsletter_on_join"`
	Leave      *events.NewsletterLeave      `json:"xwa2_notify_newsletter_on_leave"`
	MuteChange *events.NewsletterMuteChange `json:"xwa2_notify_newsletter_on_mute_change"`
	// _on_admin_metadata_update -> id, thread_metadata, messages
	// _on_metadata_update
	// _on_state_change -> id, is_requestor, state
}

func (cli *Client) handleMexNotification(ctx context.Context, node *waBinary.Node) {
	for _, child := range node.GetChildren() {
		if child.Tag != "update" {
			continue
		}
		childData, ok := child.Content.([]byte)
		if !ok {
			continue
		}
		var wrapper newsLetterEventWrapper
		err := json.Unmarshal(childData, &wrapper)
		if err != nil {
			cli.Log.Errorf("Failed to unmarshal JSON in mex event: %v", err)
			continue
		}
		if wrapper.Data.Join != nil {
			cli.dispatchEvent(wrapper.Data.Join)
		} else if wrapper.Data.Leave != nil {
			cli.dispatchEvent(wrapper.Data.Leave)
		} else if wrapper.Data.MuteChange != nil {
			cli.dispatchEvent(wrapper.Data.MuteChange)
		}
	}
}
