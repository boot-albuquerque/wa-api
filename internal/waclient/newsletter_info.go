// Copyright (c) 2023 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"encoding/json"
	"strings"

	"wa-api/internal/waclient/types"
)

type respGetNewsletterInfo struct {
	Newsletter *types.NewsletterMetadata `json:"xwa2_newsletter"`
}

func (cli *Client) getNewsletterInfo(ctx context.Context, input map[string]any, fetchViewerMeta bool) (*types.NewsletterMetadata, error) {
	data, err := cli.sendMexIQ(ctx, queryFetchNewsletter, map[string]any{
		"fetch_creation_time":   true,
		"fetch_full_image":      true,
		"fetch_viewer_metadata": fetchViewerMeta,
		"input":                 input,
	})
	var respData respGetNewsletterInfo
	if data != nil {
		jsonErr := json.Unmarshal(data, &respData)
		if err == nil && jsonErr != nil {
			err = jsonErr
		}
	}
	return respData.Newsletter, err
}

// GetNewsletterInfo gets the info of a newsletter that you're joined to.
func (cli *Client) GetNewsletterInfo(ctx context.Context, jid types.JID) (*types.NewsletterMetadata, error) {
	return cli.getNewsletterInfo(ctx, map[string]any{
		"key":  jid.String(),
		"type": types.NewsletterKeyTypeJID,
	}, true)
}

// GetNewsletterInfoWithInvite gets the info of a newsletter with an invite link.
//
// You can either pass the full link (https://whatsapp.com/channel/...) or just the `...` part.
//
// Note that the ViewerMeta field of the returned NewsletterMetadata will be nil.
func (cli *Client) GetNewsletterInfoWithInvite(ctx context.Context, key string) (*types.NewsletterMetadata, error) {
	return cli.getNewsletterInfo(ctx, map[string]any{
		"key":  strings.TrimPrefix(key, NewsletterLinkPrefix),
		"type": types.NewsletterKeyTypeInvite,
	}, false)
}

type respGetSubscribedNewsletters struct {
	Newsletters []*types.NewsletterMetadata `json:"xwa2_newsletter_subscribed"`
}

// GetSubscribedNewsletters gets the info of all newsletters that you're joined to.
func (cli *Client) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	data, err := cli.sendMexIQ(ctx, querySubscribedNewsletters, map[string]any{})
	var respData respGetSubscribedNewsletters
	if data != nil {
		jsonErr := json.Unmarshal(data, &respData)
		if err == nil && jsonErr != nil {
			err = jsonErr
		}
	}
	return respData.Newsletters, err
}
