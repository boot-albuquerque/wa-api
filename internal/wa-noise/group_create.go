// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"
	"strings"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// ReqCreateGroup contains the request data for CreateGroup.
type ReqCreateGroup struct {
	// Group names are limited to 25 characters. A longer group name will cause a 406 not acceptable error.
	Name string
	// You don't need to include your own JID in the participants array, the WhatsApp servers will add it implicitly.
	Participants []types.JID
	// A create key can be provided to deduplicate the group create notification that will be triggered
	// when the group is created. If provided, the JoinedGroup event will contain the same key.
	CreateKey types.MessageID

	types.GroupEphemeral
	types.GroupAnnounce
	types.GroupLocked
	types.GroupMembershipApprovalMode
	// Set IsParent to true to create a community instead of a normal group.
	// When creating a community, the linked announcement group will be created automatically by the server.
	types.GroupParent
	// Set LinkedParentJID to create a group inside a community.
	types.GroupLinkedParent
}

// CreateGroup creates a group on WhatsApp with the given name and participants.
//
// See ReqCreateGroup for parameters.
func (cli *Client) CreateGroup(ctx context.Context, req ReqCreateGroup) (*types.GroupInfo, error) {
	participantNodes := make([]waBinary.Node, len(req.Participants), len(req.Participants)+1)
	for i, participant := range req.Participants {
		participantNodes[i] = waBinary.Node{
			Tag:   "participant",
			Attrs: waBinary.Attrs{"jid": participant},
		}
		pt, err := cli.Store.PrivacyTokens.GetPrivacyToken(ctx, participant)
		if err != nil {
			return nil, fmt.Errorf("failed to get privacy token for participant %s: %v", participant, err)
		} else if pt != nil {
			participantNodes[i].Content = []waBinary.Node{{
				Tag:     "privacy",
				Content: pt.Token,
			}}
		}
	}
	if req.CreateKey == "" {
		req.CreateKey = cli.GenerateMessageID()
	}
	if req.IsParent {
		if req.DefaultMembershipApprovalMode == "" {
			req.DefaultMembershipApprovalMode = "request_required"
		}
		participantNodes = append(participantNodes, waBinary.Node{
			Tag: "parent",
			Attrs: waBinary.Attrs{
				"default_membership_approval_mode": req.DefaultMembershipApprovalMode,
			},
		})
	} else if !req.LinkedParentJID.IsEmpty() {
		participantNodes = append(participantNodes, waBinary.Node{
			Tag:   "linked_parent",
			Attrs: waBinary.Attrs{"jid": req.LinkedParentJID},
		})
	}
	if req.IsLocked {
		participantNodes = append(participantNodes, waBinary.Node{Tag: "locked"})
	}
	if req.IsAnnounce {
		participantNodes = append(participantNodes, waBinary.Node{Tag: "announcement"})
	}
	if req.IsEphemeral {
		participantNodes = append(participantNodes, waBinary.Node{
			Tag: "ephemeral",
			Attrs: waBinary.Attrs{
				"expiration": req.DisappearingTimer,
				"trigger":    "1", // TODO what's this?
			},
		})
	}
	if req.IsJoinApprovalRequired {
		participantNodes = append(participantNodes, waBinary.Node{
			Tag: "membership_approval_mode",
			Content: []waBinary.Node{{
				Tag:   "group_join",
				Attrs: waBinary.Attrs{"state": "on"},
			}},
		})
	}
	// WhatsApp web doesn't seem to include the static prefix for these
	key := strings.TrimPrefix(req.CreateKey, "3EB0")
	resp, err := cli.sendGroupIQ(ctx, iqSet, types.GroupServerJID, waBinary.Node{
		Tag: "create",
		Attrs: waBinary.Attrs{
			"subject": req.Name,
			"key":     key,
		},
		Content: participantNodes,
	})
	if err != nil {
		return nil, err
	}
	groupNode, ok := resp.GetOptionalChildByTag("group")
	if !ok {
		return nil, &ElementMissingError{Tag: "group", In: "response to create group query"}
	}
	return cli.parseGroupNode(&groupNode)
}

// UnlinkGroup removes a child group from a parent community.
func (cli *Client) UnlinkGroup(ctx context.Context, parent, child types.JID) error {
	_, err := cli.sendGroupIQ(ctx, iqSet, parent, waBinary.Node{
		Tag:   "unlink",
		Attrs: waBinary.Attrs{"unlink_type": string(types.GroupLinkChangeTypeSub)},
		Content: []waBinary.Node{{
			Tag:   "group",
			Attrs: waBinary.Attrs{"jid": child},
		}},
	})
	return err
}

// LinkGroup adds an existing group as a child group in a community.
//
// To create a new group within a community, set LinkedParentJID in the CreateGroup request.
func (cli *Client) LinkGroup(ctx context.Context, parent, child types.JID) error {
	_, err := cli.sendGroupIQ(ctx, iqSet, parent, waBinary.Node{
		Tag: "links",
		Content: []waBinary.Node{{
			Tag:   "link",
			Attrs: waBinary.Attrs{"link_type": string(types.GroupLinkChangeTypeSub)},
			Content: []waBinary.Node{{
				Tag:   "group",
				Attrs: waBinary.Attrs{"jid": child},
			}},
		}},
	})
	return err
}

// LeaveGroup leaves the specified group on WhatsApp.
func (cli *Client) LeaveGroup(ctx context.Context, jid types.JID) error {
	_, err := cli.sendGroupIQ(ctx, iqSet, types.GroupServerJID, waBinary.Node{
		Tag: "leave",
		Content: []waBinary.Node{{
			Tag:   "group",
			Attrs: waBinary.Attrs{"id": jid},
		}},
	})
	return err
}
