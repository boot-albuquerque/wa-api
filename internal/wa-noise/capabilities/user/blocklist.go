// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package user

import (
	"context"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// ParseBlocklist le o <list> da resposta de blocklist.
//
// A tag dos filhos nao e' conferida: o criterio de aceitacao e' o `jid` ser
// lido com sucesso.
func ParseBlocklist(t Transport, node *waBinary.Node) *types.Blocklist {
	output := &types.Blocklist{
		DHash: node.AttrGetter().String("dhash"),
	}
	for _, child := range node.GetChildren() {
		ag := child.AttrGetter()
		blockedJID := ag.JID("jid")
		if !ag.OK() {
			t.Log().Debugf("Ignoring contact blocked data with unexpected attributes: %v", ag.Error())
			continue
		}

		output.JIDs = append(output.JIDs, blockedJID)
	}
	return output
}

// GetBlocklist gets the list of users that this user has blocked.
func GetBlocklist(ctx context.Context, t Transport) (*types.Blocklist, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: blocklistIQNamespace,
		Type:      IQGet,
		To:        types.ServerJID,
	})
	if err != nil {
		return nil, err
	}
	list, ok := resp.GetOptionalChildByTag("list")
	if !ok {
		return nil, t.ElementMissing("list", "response to blocklist query")
	}
	return ParseBlocklist(t, &list), nil
}

// UpdateBlocklist updates the user's block list and returns the updated list.
func UpdateBlocklist(
	ctx context.Context, t Transport,
	jid types.JID, action events.BlocklistChangeAction,
) (*types.Blocklist, error) {
	resp, err := t.SendIQ(ctx, IQ{
		Namespace: blocklistIQNamespace,
		Type:      IQSet,
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
		return nil, t.ElementMissing("list", "response to blocklist update")
	}
	return ParseBlocklist(t, &list), err
}
