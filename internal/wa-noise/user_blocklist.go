// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

func (cli *Client) parseBlocklist(node *waBinary.Node) *types.Blocklist {
	output := &types.Blocklist{
		DHash: node.AttrGetter().String("dhash"),
	}
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		blockedJID := ag.JID("jid")
		if !ag.OK() {
			cli.Log.Debugf("Ignoring contact blocked data with unexpected attributes: %v", ag.Error())
			continue
		}

		output.JIDs = append(output.JIDs, blockedJID)
	}
	return output
}

// GetBlocklist gets the list of users that this user has blocked.
func (cli *Client) GetBlocklist(ctx context.Context) (*types.Blocklist, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "blocklist",
		Type:      iqGet,
		To:        types.ServerJID,
	})
	if err != nil {
		return nil, err
	}
	list, ok := resp.GetOptionalChildByTag("list")
	if !ok {
		return nil, &ElementMissingError{Tag: "list", In: "response to blocklist query"}
	}
	return cli.parseBlocklist(&list), nil
}

// UpdateBlocklist updates the user's block list and returns the updated list.
func (cli *Client) UpdateBlocklist(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
	resp, err := cli.sendIQ(ctx, infoQuery{
		Namespace: "blocklist",
		Type:      iqSet,
		To:        types.ServerJID,
		Content: []waBinary.Node{{
			Tag: "item",
			Attrs: waBinary.Attrs{
				"jid":    jid,
				"action": string(action),
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	list, ok := resp.GetOptionalChildByTag("list")
	if !ok {
		return nil, &ElementMissingError{Tag: "list", In: "response to blocklist update"}
	}
	return cli.parseBlocklist(&list), err
}
