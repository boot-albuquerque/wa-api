// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

const InviteLinkPrefix = "https://chat.whatsapp.com/"

// GetGroupInviteLink requests the invite link to the group from the WhatsApp servers.
//
// If reset is true, then the old invite link will be revoked and a new one generated.
func (cli *Client) GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error) {
	iqType := iqGet
	if reset {
		iqType = iqSet
	}
	resp, err := cli.sendGroupIQ(ctx, iqType, jid, waBinary.Node{Tag: groupInviteTag})
	if errors.Is(err, ErrIQNotAuthorized) {
		return "", wrapIQError(ErrGroupInviteLinkUnauthorized, err)
	} else if errors.Is(err, ErrIQNotFound) {
		return "", wrapIQError(ErrGroupNotFound, err)
	} else if errors.Is(err, ErrIQForbidden) {
		return "", wrapIQError(ErrNotInGroup, err)
	} else if err != nil {
		return "", err
	}
	code, ok := resp.GetChildByTag(groupInviteTag).Attrs["code"].(string)
	if !ok {
		return "", fmt.Errorf("didn't find invite code in response")
	}
	return InviteLinkPrefix + code, nil
}

// GetGroupInfoFromInvite gets the group info from an invite message.
//
// Note that this is specifically for invite messages, not invite links. Use GetGroupInfoFromLink for resolving chat.whatsapp.com links.
func (cli *Client) GetGroupInfoFromInvite(ctx context.Context, jid, inviter types.JID, code string, expiration int64) (*types.GroupInfo, error) {
	resp, err := cli.sendGroupIQ(ctx, iqGet, jid, waBinary.Node{
		Tag: "query",
		Content: []waBinary.Node{{
			Tag: groupAddRequestTag,
			Attrs: waBinary.Attrs{
				"code":       code,
				"expiration": expiration,
				"admin":      inviter,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	groupNode, ok := resp.GetOptionalChildByTag(groupNodeTag)
	if !ok {
		return nil, &ElementMissingError{Tag: groupNodeTag, In: "response to invite group info query"}
	}
	return cli.parseGroupNode(&groupNode)
}

// JoinGroupWithInvite joins a group using an invite message.
//
// Note that this is specifically for invite messages, not invite links. Use JoinGroupWithLink for joining with chat.whatsapp.com links.
func (cli *Client) JoinGroupWithInvite(ctx context.Context, jid, inviter types.JID, code string, expiration int64) error {
	_, err := cli.sendGroupIQ(ctx, iqSet, jid, waBinary.Node{
		Tag: "accept",
		Attrs: waBinary.Attrs{
			"code":       code,
			"expiration": expiration,
			"admin":      inviter,
		},
	})
	return err
}

// GetGroupInfoFromLink resolves the given invite link and asks the WhatsApp servers for info about the group.
// This will not cause the user to join the group.
func (cli *Client) GetGroupInfoFromLink(ctx context.Context, code string) (*types.GroupInfo, error) {
	code = strings.TrimPrefix(code, InviteLinkPrefix)
	resp, err := cli.sendGroupIQ(ctx, iqGet, types.GroupServerJID, waBinary.Node{
		Tag:   groupInviteTag,
		Attrs: waBinary.Attrs{"code": code},
	})
	if errors.Is(err, ErrIQGone) {
		return nil, wrapIQError(ErrInviteLinkRevoked, err)
	} else if errors.Is(err, ErrIQNotAcceptable) {
		return nil, wrapIQError(ErrInviteLinkInvalid, err)
	} else if err != nil {
		return nil, err
	}
	groupNode, ok := resp.GetOptionalChildByTag(groupNodeTag)
	if !ok {
		return nil, &ElementMissingError{Tag: groupNodeTag, In: "response to group link info query"}
	}
	return cli.parseGroupNode(&groupNode)
}

// JoinGroupWithLink joins the group using the given invite link.
func (cli *Client) JoinGroupWithLink(ctx context.Context, code string) (types.JID, error) {
	code = strings.TrimPrefix(code, InviteLinkPrefix)
	resp, err := cli.sendGroupIQ(ctx, iqSet, types.GroupServerJID, waBinary.Node{
		Tag:   groupInviteTag,
		Attrs: waBinary.Attrs{"code": code},
	})
	if errors.Is(err, ErrIQGone) {
		return types.EmptyJID, wrapIQError(ErrInviteLinkRevoked, err)
	} else if errors.Is(err, ErrIQNotAcceptable) {
		return types.EmptyJID, wrapIQError(ErrInviteLinkInvalid, err)
	} else if err != nil {
		return types.EmptyJID, err
	}
	membershipApprovalModeNode, ok := resp.GetOptionalChildByTag("membership_approval_request")
	if ok {
		return membershipApprovalModeNode.AttrGetter().JID("jid"), nil
	}
	groupNode, ok := resp.GetOptionalChildByTag(groupNodeTag)
	if !ok {
		return types.EmptyJID, &ElementMissingError{Tag: groupNodeTag, In: "response to group link join query"}
	}
	return groupNode.AttrGetter().JID("jid"), nil
}
