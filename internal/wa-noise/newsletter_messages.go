// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

type GetNewsletterMessagesParams struct {
	Count  int
	Before types.MessageServerID
}

// newsletterMessagesAttrs monta os atributos do nó <messages>. Params nil ou
// com campos zerados omite o atributo correspondente, deixando o servidor usar
// o padrão dele.
func newsletterMessagesAttrs(jid types.JID, params *GetNewsletterMessagesParams) waBinary.Attrs {
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
	return attrs
}

// GetNewsletterMessages gets messages in a WhatsApp channel.
func (cli *Client) GetNewsletterMessages(ctx context.Context, jid types.JID, params *GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: newsletterNamespace,
		Type:      iqGet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag:   newsletterMessagesTag,
			Attrs: newsletterMessagesAttrs(jid, params),
		}},
	})
	if err != nil {
		return nil, err
	}
	messages, ok := resp.GetOptionalChildByTag(newsletterMessagesTag)
	if !ok {
		return nil, &ElementMissingError{Tag: newsletterMessagesTag, In: newsletterMessagesErrContext}
	}
	return cli.parseNewsletterMessages(&messages), nil
}

type GetNewsletterUpdatesParams struct {
	Count int
	Since time.Time
	After types.MessageServerID
}

// newsletterMessageUpdatesAttrs monta os atributos do nó <message_updates>.
// Assim como em newsletterMessagesAttrs, campo zerado vira atributo ausente.
func newsletterMessageUpdatesAttrs(params *GetNewsletterUpdatesParams) waBinary.Attrs {
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
	return attrs
}

// GetNewsletterMessageUpdates gets updates in a WhatsApp channel.
//
// These are the same kind of updates that NewsletterSubscribeLiveUpdates triggers (reaction and view counts).
func (cli *Client) GetNewsletterMessageUpdates(ctx context.Context, jid types.JID, params *GetNewsletterUpdatesParams) ([]*types.NewsletterMessage, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: newsletterNamespace,
		Type:      iqGet,
		To:        jid,
		Content: []waBinary.Node{{
			Tag:   newsletterMessageUpdatesTag,
			Attrs: newsletterMessageUpdatesAttrs(params),
		}},
	})
	if err != nil {
		return nil, err
	}
	messages, ok := resp.GetOptionalChildByTag(newsletterMessageUpdatesTag, newsletterMessagesTag)
	if !ok {
		return nil, &ElementMissingError{Tag: newsletterMessagesTag, In: newsletterMessagesErrContext}
	}
	return cli.parseNewsletterMessages(&messages), nil
}
