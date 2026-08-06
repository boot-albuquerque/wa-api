// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"time"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

type GetNewsletterMessagesParams struct {
	Count  int
	Before types.MessageServerID
}

// GetNewsletterMessages gets messages in a WhatsApp channel.
func (cli *Client) GetNewsletterMessages(ctx context.Context, jid types.JID, params *GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error) {
	attrs := waBinary.Attrs{
		"type": "jid",
		"jid":  jid,
	}
	if params != nil {
		if params.Count != 0 {
			attrs["count"] = params.Count
		}
		if params.Before != 0 {
			attrs["before"] = params.Before
		}
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "newsletter",
		Type:      iqGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:   "messages",
			Attrs: attrs,
		}},
	})
	if err != nil {
		return nil, err
	}
	messages, ok := resp.GetOptionalChildByTag("messages")
	if !ok {
		return nil, &ElementMissingError{Tag: "messages", In: "newsletter messages response"}
	}
	return cli.parseNewsletterMessages(&messages), nil
}

type GetNewsletterUpdatesParams struct {
	Count int
	Since time.Time
	After types.MessageServerID
}

// GetNewsletterMessageUpdates gets updates in a WhatsApp channel.
//
// These are the same kind of updates that NewsletterSubscribeLiveUpdates triggers (reaction and view counts).
func (cli *Client) GetNewsletterMessageUpdates(ctx context.Context, jid types.JID, params *GetNewsletterUpdatesParams) ([]*types.NewsletterMessage, error) {
	attrs := waBinary.Attrs{}
	if params != nil {
		if params.Count != 0 {
			attrs["count"] = params.Count
		}
		if !params.Since.IsZero() {
			attrs["since"] = params.Since.Unix()
		}
		if params.After != 0 {
			attrs["after"] = params.After
		}
	}
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "newsletter",
		Type:      iqGet,
		To:        jid,
		Content: []waBinary.Node{{
			Tag:   "message_updates",
			Attrs: attrs,
		}},
	})
	if err != nil {
		return nil, err
	}
	messages, ok := resp.GetOptionalChildByTag("message_updates", "messages")
	if !ok {
		return nil, &ElementMissingError{Tag: "messages", In: "newsletter messages response"}
	}
	return cli.parseNewsletterMessages(&messages), nil
}
