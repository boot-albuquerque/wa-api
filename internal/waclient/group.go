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

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/store"
	"wa-api/internal/waclient/types"
)

func (cli *Client) sendGroupIQ(ctx context.Context, iqType infoQueryType, jid types.JID, content waBinary.Node) (*waBinary.Node, error) {
	return cli.sendIQ(ctx, infoQuery{
		Namespace: "w:g2",
		Type:      iqType,
		To:        jid,
		Content:   []waBinary.Node{content},
	})
}

// GetJoinedGroups returns the list of groups the user is participating in.
func (cli *Client) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	resp, err := cli.sendGroupIQ(ctx, iqGet, types.GroupServerJID, waBinary.Node{
		Tag: "participating",
		Content: []waBinary.Node{
			{Tag: "participants"},
			{Tag: "description"},
		},
	})
	if err != nil {
		return nil, err
	}
	groups, ok := resp.GetOptionalChildByTag("groups")
	if !ok {
		return nil, &ElementMissingError{Tag: "groups", In: "response to group list query"}
	}
	children := groups.GetChildren()
	infos := make([]*types.GroupInfo, 0, len(children))
	var allLIDPairs []store.LIDMapping
	var allRedactedPhones []store.RedactedPhoneEntry
	for _, child := range children {
		if child.Tag != "group" {
			cli.Log.Debugf("Unexpected child in group list response: %s", child.XMLString())
			continue
		}
		parsed, parseErr := cli.parseGroupNode(&child)
		if parseErr != nil {
			cli.Log.Warnf("Error parsing group %s: %v", parsed.JID, parseErr)
		}
		lidPairs, redactedPhones := cli.cacheGroupInfo(parsed, true)
		allLIDPairs = append(allLIDPairs, lidPairs...)
		allRedactedPhones = append(allRedactedPhones, redactedPhones...)
		infos = append(infos, parsed)
	}
	err = cli.Store.LIDs.PutManyLIDMappings(ctx, allLIDPairs)
	if err != nil {
		cli.Log.Warnf("Failed to store LID mappings from joined groups: %v", err)
	}
	err = cli.Store.Contacts.PutManyRedactedPhones(ctx, allRedactedPhones)
	if err != nil {
		cli.Log.Warnf("Failed to store redacted phones from joined groups: %v", err)
	}
	return infos, nil
}

// GetSubGroups gets the subgroups of the given community.
func (cli *Client) GetSubGroups(ctx context.Context, community types.JID) ([]*types.GroupLinkTarget, error) {
	res, err := cli.sendGroupIQ(ctx, iqGet, community, waBinary.Node{Tag: "sub_groups"})
	if err != nil {
		return nil, err
	}
	groups, ok := res.GetOptionalChildByTag("sub_groups")
	if !ok {
		return nil, &ElementMissingError{Tag: "sub_groups", In: "response to subgroups query"}
	}
	var parsedGroups []*types.GroupLinkTarget
	for _, child := range groups.GetChildren() {
		if child.Tag == "group" {
			parsedGroup, err := parseGroupLinkTargetNode(&child)
			if err != nil {
				return parsedGroups, fmt.Errorf("failed to parse group in subgroups list: %w", err)
			}
			parsedGroups = append(parsedGroups, &parsedGroup)
		}
	}
	return parsedGroups, nil
}

// GetLinkedGroupsParticipants gets all the participants in the groups of the given community.
func (cli *Client) GetLinkedGroupsParticipants(ctx context.Context, community types.JID) ([]types.JID, error) {
	res, err := cli.sendGroupIQ(ctx, iqGet, community, waBinary.Node{Tag: "linked_groups_participants"})
	if err != nil {
		return nil, err
	}
	participants, ok := res.GetOptionalChildByTag("linked_groups_participants")
	if !ok {
		return nil, &ElementMissingError{Tag: "linked_groups_participants", In: "response to community participants query"}
	}
	members, lidPairs := parseParticipantList(&participants)
	if len(lidPairs) > 0 {
		err = cli.Store.LIDs.PutManyLIDMappings(ctx, lidPairs)
		if err != nil {
			cli.Log.Warnf("Failed to store LID mappings for community participants: %v", err)
		}
	}
	return members, nil
}

// GetGroupInfo requests basic info about a group chat from the WhatsApp servers.
func (cli *Client) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	return cli.getGroupInfo(ctx, jid, true)
}

func (cli *Client) cacheGroupInfo(groupInfo *types.GroupInfo, lock bool) ([]store.LIDMapping, []store.RedactedPhoneEntry) {
	participants := make([]types.JID, len(groupInfo.Participants))
	lidPairs := make([]store.LIDMapping, len(groupInfo.Participants))
	redactedPhones := make([]store.RedactedPhoneEntry, 0)
	for i, part := range groupInfo.Participants {
		participants[i] = part.JID
		if !part.PhoneNumber.IsEmpty() && !part.LID.IsEmpty() {
			lidPairs[i] = store.LIDMapping{
				LID: part.LID,
				PN:  part.PhoneNumber,
			}
		}
		if part.DisplayName != "" && !part.LID.IsEmpty() {
			redactedPhones = append(redactedPhones, store.RedactedPhoneEntry{
				JID:           part.LID,
				RedactedPhone: part.DisplayName,
			})
		}
	}
	if lock {
		cli.groupCacheLock.Lock()
		defer cli.groupCacheLock.Unlock()
	}
	cli.groupCache[groupInfo.JID] = &groupMetaCache{
		AddressingMode:             groupInfo.AddressingMode,
		CommunityAnnouncementGroup: groupInfo.IsAnnounce && groupInfo.IsDefaultSubGroup,
		Members:                    participants,
	}
	return lidPairs, redactedPhones
}

func (cli *Client) getGroupInfo(ctx context.Context, jid types.JID, lockParticipantCache bool) (*types.GroupInfo, error) {
	res, err := cli.sendGroupIQ(ctx, iqGet, jid, waBinary.Node{
		Tag:   "query",
		Attrs: waBinary.Attrs{"request": "interactive"},
	})
	if errors.Is(err, ErrIQNotFound) {
		return nil, wrapIQError(ErrGroupNotFound, err)
	} else if errors.Is(err, ErrIQForbidden) {
		return nil, wrapIQError(ErrNotInGroup, err)
	} else if err != nil {
		return nil, err
	}

	groupNode, ok := res.GetOptionalChildByTag("group")
	if !ok {
		return nil, &ElementMissingError{Tag: "groups", In: "response to group info query"}
	}
	groupInfo, err := cli.parseGroupNode(&groupNode)
	if err != nil {
		return groupInfo, err
	}
	lidPairs, redactedPhones := cli.cacheGroupInfo(groupInfo, lockParticipantCache)
	err = cli.Store.LIDs.PutManyLIDMappings(ctx, lidPairs)
	if err != nil {
		cli.Log.Warnf("Failed to store LID mappings for members of %s: %v", jid, err)
	}
	err = cli.Store.Contacts.PutManyRedactedPhones(ctx, redactedPhones)
	if err != nil {
		cli.Log.Warnf("Failed to store redacted phones for members of %s: %v", jid, err)
	}
	return groupInfo, nil
}

func (cli *Client) getCachedGroupData(ctx context.Context, jid types.JID) (*groupMetaCache, error) {
	cli.groupCacheLock.Lock()
	defer cli.groupCacheLock.Unlock()
	if val, ok := cli.groupCache[jid]; ok {
		return val, nil
	}
	_, err := cli.getGroupInfo(ctx, jid, false)
	if err != nil {
		return nil, err
	}
	return cli.groupCache[jid], nil
}
