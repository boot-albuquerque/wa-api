// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/json"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// NewsletterSubscribeLiveUpdates subscribes to receive live updates from a WhatsApp channel temporarily (for the duration returned).
func (cli *Client) NewsletterSubscribeLiveUpdates(ctx context.Context, jid types.JID) (time.Duration, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "newsletter",
		Type:      iqSet,
		To:        jid,
		Content: []waBinary.Node{{
			Tag: "live_updates",
		}},
	})
	if err != nil {
		return 0, err
	}
	child := resp.GetChildByTag("live_updates")
	dur := child.AttrGetter().Int("duration")
	return time.Duration(dur) * time.Second, nil
}

// NewsletterMarkViewed marks a channel message as viewed, incrementing the view counter.
//
// This is not the same as marking the channel as read on your other devices, use the usual MarkRead function for that.
func (cli *Client) NewsletterMarkViewed(ctx context.Context, jid types.JID, serverIDs []types.MessageServerID) error {
	if cli == nil {
		return ErrClientIsNil
	}
	items := make([]waBinary.Node, len(serverIDs))
	for i, id := range serverIDs {
		items[i] = waBinary.Node{
			Tag: "item",
			Attrs: waBinary.Attrs{
				"server_id": id,
			},
		}
	}
	reqID := cli.generateRequestID()
	resp := cli.waitResponse(reqID)
	err := cli.sendNode(ctx, waBinary.Node{
		Tag: "receipt",
		Attrs: waBinary.Attrs{
			"to":   jid,
			"type": "view",
			"id":   reqID,
		},
		Content: []waBinary.Node{{
			Tag:     "list",
			Content: items,
		}},
	})
	if err != nil {
		cli.cancelResponse(reqID, resp)
		return err
	}
	// TODO handle response?
	<-resp
	return nil
}

// NewsletterSendReaction sends a reaction to a channel message.
// To remove a reaction sent earlier, set reaction to an empty string.
//
// The last parameter is the message ID of the reaction itself. It can be left empty to let whatsmeow generate a random one.
func (cli *Client) NewsletterSendReaction(ctx context.Context, jid types.JID, serverID types.MessageServerID, reaction string, messageID types.MessageID) error {
	if messageID == "" {
		messageID = cli.GenerateMessageID()
	}
	reactionAttrs := waBinary.Attrs{}
	messageAttrs := waBinary.Attrs{
		"to":        jid,
		"id":        messageID,
		"server_id": serverID,
		"type":      "reaction",
	}
	if reaction != "" {
		reactionAttrs["code"] = reaction
	} else {
		messageAttrs["edit"] = string(types.EditAttributeSenderRevoke)
	}
	return cli.sendNode(ctx, waBinary.Node{
		Tag:   "message",
		Attrs: messageAttrs,
		Content: []waBinary.Node{{
			Tag:   "reaction",
			Attrs: reactionAttrs,
		}},
	})
}

type CreateNewsletterParams struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Picture     []byte `json:"picture,omitempty"`
}

type respCreateNewsletter struct {
	Newsletter *types.NewsletterMetadata `json:"xwa2_newsletter_create"`
}

// CreateNewsletter creates a new WhatsApp channel.
func (cli *Client) CreateNewsletter(ctx context.Context, params CreateNewsletterParams) (*types.NewsletterMetadata, error) {
	resp, err := cli.sendMexIQ(ctx, mutationCreateNewsletter, map[string]any{
		"newsletter_input": &params,
	})
	if err != nil {
		return nil, err
	}
	var respData respCreateNewsletter
	err = json.Unmarshal(resp, &respData)
	if err != nil {
		return nil, err
	}
	return respData.Newsletter, nil
}

// AcceptTOSNotice accepts a ToS notice.
//
// To accept the terms for creating newsletters, use
//
//	cli.AcceptTOSNotice("20601218", "5")
func (cli *Client) AcceptTOSNotice(ctx context.Context, noticeID, stage string) error {
	_, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "tos",
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "notice",
			Attrs: waBinary.Attrs{
				"id":    noticeID,
				"stage": stage,
			},
		}},
	})
	return err
}

// NewsletterToggleMute changes the mute status of a newsletter.
func (cli *Client) NewsletterToggleMute(ctx context.Context, jid types.JID, mute bool) error {
	query := mutationUnmuteNewsletter
	if mute {
		query = mutationMuteNewsletter
	}
	_, err := cli.sendMexIQ(ctx, query, map[string]any{
		"newsletter_id": jid.String(),
	})
	return err
}

// FollowNewsletter makes the user follow (join) a WhatsApp channel.
func (cli *Client) FollowNewsletter(ctx context.Context, jid types.JID) error {
	_, err := cli.sendMexIQ(ctx, mutationFollowNewsletter, map[string]any{
		"newsletter_id": jid.String(),
	})
	return err
}

// UnfollowNewsletter makes the user unfollow (leave) a WhatsApp channel.
func (cli *Client) UnfollowNewsletter(ctx context.Context, jid types.JID) error {
	_, err := cli.sendMexIQ(ctx, mutationUnfollowNewsletter, map[string]any{
		"newsletter_id": jid.String(),
	})
	return err
}
