// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"fmt"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

func (cli *Client) parseGroupCreate(parentNode, node *waBinary.Node) (*events.JoinedGroup, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	groupNode, ok := node.GetOptionalChildByTag("group")
	if !ok {
		return nil, nil, nil, fmt.Errorf("group create notification didn't contain group info")
	}
	var evt events.JoinedGroup
	pag := parentNode.AttrGetter()
	ag := node.AttrGetter()
	evt.Reason = ag.OptionalString("reason")
	evt.CreateKey = ag.OptionalString("key")
	evt.Type = ag.OptionalString("type")
	evt.Sender = pag.OptionalJID("participant")
	evt.SenderPN = pag.OptionalJID("participant_pn")
	evt.Notify = pag.OptionalString("notify")
	info, err := cli.parseGroupNode(&groupNode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse group info in create notification: %w", err)
	}
	evt.GroupInfo = *info
	lidPairs, redactedPhones := cli.cacheGroupInfo(info, true)
	return &evt, lidPairs, redactedPhones, nil
}

func (cli *Client) parseGroupChange(node *waBinary.Node) (*events.GroupInfo, []store.LIDMapping, error) {
	var evt events.GroupInfo
	ag := node.AttrGetter()
	evt.JID = ag.JID("from")
	evt.Notify = ag.OptionalString("notify")
	evt.Sender = ag.OptionalJID("participant")
	evt.SenderPN = ag.OptionalJID("participant_pn")
	evt.Timestamp = ag.UnixTime("t")
	if !ag.OK() {
		return nil, nil, fmt.Errorf("group change doesn't contain required attributes: %w", ag.Error())
	}

	var lidPairs []store.LIDMapping
	for _, child := range node.GetChildren() {
		cag := child.AttrGetter()
		if child.Tag == "add" || child.Tag == "remove" || child.Tag == "promote" || child.Tag == "demote" {
			evt.PrevParticipantVersionID = cag.OptionalString("prev_v_id")
			evt.ParticipantVersionID = cag.OptionalString("v_id")
		}
		switch child.Tag {
		case "add":
			evt.JoinReason = cag.OptionalString("reason")
			evt.Join, lidPairs = parseParticipantList(&child)
		case "remove":
			evt.Leave, lidPairs = parseParticipantList(&child)
		case "promote":
			evt.Promote, lidPairs = parseParticipantList(&child)
		case "demote":
			evt.Demote, lidPairs = parseParticipantList(&child)
		case "locked":
			evt.Locked = &types.GroupLocked{IsLocked: true}
		case "unlocked":
			evt.Locked = &types.GroupLocked{IsLocked: false}
		case "delete":
			evt.Delete = &types.GroupDelete{Deleted: true, DeleteReason: cag.String("reason")}
		case "subject":
			evt.Name = &types.GroupName{
				Name:        cag.String("subject"),
				NameSetAt:   cag.UnixTime("s_t"),
				NameSetBy:   cag.OptionalJIDOrEmpty("s_o"),
				NameSetByPN: cag.OptionalJIDOrEmpty("s_o_pn"),
			}
		case "description":
			var topicStr string
			_, isDelete := child.GetOptionalChildByTag("delete")
			if !isDelete {
				topicChild := child.GetChildByTag("body")
				topicBytes, ok := topicChild.Content.([]byte)
				if !ok {
					return nil, nil, fmt.Errorf("group change description has unexpected body: %s", topicChild.XMLString())
				}
				topicStr = string(topicBytes)
			}
			var setBy types.JID
			if evt.Sender != nil {
				setBy = *evt.Sender
			}
			evt.Topic = &types.GroupTopic{
				Topic:        topicStr,
				TopicID:      cag.String("id"),
				TopicSetAt:   evt.Timestamp,
				TopicSetBy:   setBy,
				TopicDeleted: isDelete,
			}
		case "announcement":
			evt.Announce = &types.GroupAnnounce{
				IsAnnounce:        true,
				AnnounceVersionID: cag.String("v_id"),
			}
		case "not_announcement":
			evt.Announce = &types.GroupAnnounce{
				IsAnnounce:        false,
				AnnounceVersionID: cag.String("v_id"),
			}
		case "invite":
			link := InviteLinkPrefix + cag.String("code")
			evt.NewInviteLink = &link
		case "ephemeral":
			timer := uint32(cag.Uint64("expiration"))
			evt.Ephemeral = &types.GroupEphemeral{
				IsEphemeral:       true,
				DisappearingTimer: timer,
			}
		case "not_ephemeral":
			evt.Ephemeral = &types.GroupEphemeral{IsEphemeral: false}
		case "link":
			evt.Link = &types.GroupLinkChange{
				Type: types.GroupLinkChangeType(cag.String("link_type")),
			}
			groupNode, ok := child.GetOptionalChildByTag("group")
			if !ok {
				return nil, nil, &ElementMissingError{Tag: "group", In: "group link"}
			}
			var err error
			evt.Link.Group, err = parseGroupLinkTargetNode(&groupNode)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse group link node in group change: %w", err)
			}
		case "unlink":
			evt.Unlink = &types.GroupLinkChange{
				Type:         types.GroupLinkChangeType(cag.String("unlink_type")),
				UnlinkReason: types.GroupUnlinkReason(cag.String("unlink_reason")),
			}
			groupNode, ok := child.GetOptionalChildByTag("group")
			if !ok {
				return nil, nil, &ElementMissingError{Tag: "group", In: "group unlink"}
			}
			var err error
			evt.Unlink.Group, err = parseGroupLinkTargetNode(&groupNode)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse group unlink node in group change: %w", err)
			}
		case "membership_approval_mode":
			evt.MembershipApprovalMode = &types.GroupMembershipApprovalMode{
				IsJoinApprovalRequired: true,
			}
		case "suspended":
			evt.Suspended = true
		case "unsuspended":
			evt.Unsuspended = true
		default:
			evt.UnknownChanges = append(evt.UnknownChanges, &child)
		}
		if !cag.OK() {
			return nil, nil, fmt.Errorf("group change %s element doesn't contain required attributes: %w", child.Tag, cag.Error())
		}
	}
	return &evt, lidPairs, nil
}

func (cli *Client) updateGroupParticipantCache(evt *events.GroupInfo) {
	// TODO can the addressing mode change here?
	if len(evt.Join) == 0 && len(evt.Leave) == 0 {
		return
	}
	cli.groupCacheLock.Lock()
	defer cli.groupCacheLock.Unlock()
	cached, ok := cli.groupCache[evt.JID]
	if !ok {
		return
	}
Outer:
	for _, jid := range evt.Join {
		for _, existingJID := range cached.Members {
			if jid == existingJID {
				continue Outer
			}
		}
		cached.Members = append(cached.Members, jid)
	}
	for _, jid := range evt.Leave {
		for i, existingJID := range cached.Members {
			if existingJID == jid {
				cached.Members[i] = cached.Members[len(cached.Members)-1]
				cached.Members = cached.Members[:len(cached.Members)-1]
				break
			}
		}
	}
}

func (cli *Client) parseGroupNotification(node *waBinary.Node) (any, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	children := node.GetChildren()
	if len(children) == 1 && children[0].Tag == "create" {
		return cli.parseGroupCreate(node, &children[0])
	} else {
		groupChange, lidPairs, err := cli.parseGroupChange(node)
		if err != nil {
			return nil, nil, nil, err
		}
		cli.updateGroupParticipantCache(groupChange)
		return groupChange, lidPairs, nil, nil
	}
}
