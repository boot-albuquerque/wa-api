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
	groupNode, ok := node.GetOptionalChildByTag(groupNodeTag)
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

// collectParticipantList le a lista de participantes de child e **acumula** os
// pares LID/PN encontrados em lidPairs, em vez de substituir o que ja' estava
// la'. Um mesmo <notification type="w:gp2"> pode trazer mais de um elemento de
// participante (por exemplo <add> e <remove> na mesma notificacao), e os pares
// de todos eles precisam chegar ao PutManyLIDMappings do chamador.
func collectParticipantList(child *waBinary.Node, lidPairs *[]store.LIDMapping) []types.JID {
	participants, childPairs := parseParticipantList(child)
	*lidPairs = append(*lidPairs, childPairs...)
	return participants
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
		switch ParticipantChange(child.Tag) {
		case ParticipantChangeAdd, ParticipantChangeRemove, ParticipantChangePromote, ParticipantChangeDemote:
			evt.PrevParticipantVersionID = cag.OptionalString("prev_v_id")
			evt.ParticipantVersionID = cag.OptionalString("v_id")
		}
		switch child.Tag {
		case string(ParticipantChangeAdd):
			evt.JoinReason = cag.OptionalString("reason")
			evt.Join = collectParticipantList(&child, &lidPairs)
		case string(ParticipantChangeRemove):
			evt.Leave = collectParticipantList(&child, &lidPairs)
		case string(ParticipantChangePromote):
			evt.Promote = collectParticipantList(&child, &lidPairs)
		case string(ParticipantChangeDemote):
			evt.Demote = collectParticipantList(&child, &lidPairs)
		case groupLockedTag:
			evt.Locked = &types.GroupLocked{IsLocked: true}
		case groupUnlockedTag:
			evt.Locked = &types.GroupLocked{IsLocked: false}
		case groupDeleteTag:
			evt.Delete = &types.GroupDelete{Deleted: true, DeleteReason: cag.String("reason")}
		case groupSubjectTag:
			evt.Name = &types.GroupName{
				Name:        cag.String("subject"),
				NameSetAt:   cag.UnixTime("s_t"),
				NameSetBy:   cag.OptionalJIDOrEmpty("s_o"),
				NameSetByPN: cag.OptionalJIDOrEmpty("s_o_pn"),
			}
		case groupDescriptionTag:
			var topicStr string
			_, isDelete := child.GetOptionalChildByTag(groupDeleteTag)
			if !isDelete {
				topicChild := child.GetChildByTag(groupDescriptionBodyTag)
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
		case groupAnnouncementTag:
			evt.Announce = &types.GroupAnnounce{
				IsAnnounce:        true,
				AnnounceVersionID: cag.String("v_id"),
			}
		case groupNotAnnouncementTag:
			evt.Announce = &types.GroupAnnounce{
				IsAnnounce:        false,
				AnnounceVersionID: cag.String("v_id"),
			}
		case groupInviteTag:
			link := InviteLinkPrefix + cag.String("code")
			evt.NewInviteLink = &link
		case groupEphemeralTag:
			timer := uint32(cag.Uint64("expiration"))
			evt.Ephemeral = &types.GroupEphemeral{
				IsEphemeral:       true,
				DisappearingTimer: timer,
			}
		case groupNotEphemeralTag:
			evt.Ephemeral = &types.GroupEphemeral{IsEphemeral: false}
		case groupLinkTag:
			evt.Link = &types.GroupLinkChange{
				Type: types.GroupLinkChangeType(cag.String("link_type")),
			}
			groupNode, ok := child.GetOptionalChildByTag(groupNodeTag)
			if !ok {
				return nil, nil, &ElementMissingError{Tag: groupNodeTag, In: "group link"}
			}
			var err error
			evt.Link.Group, err = parseGroupLinkTargetNode(&groupNode)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse group link node in group change: %w", err)
			}
		case groupUnlinkTag:
			evt.Unlink = &types.GroupLinkChange{
				Type:         types.GroupLinkChangeType(cag.String("unlink_type")),
				UnlinkReason: types.GroupUnlinkReason(cag.String("unlink_reason")),
			}
			groupNode, ok := child.GetOptionalChildByTag(groupNodeTag)
			if !ok {
				return nil, nil, &ElementMissingError{Tag: groupNodeTag, In: "group unlink"}
			}
			var err error
			evt.Unlink.Group, err = parseGroupLinkTargetNode(&groupNode)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse group unlink node in group change: %w", err)
			}
		case groupMembershipApprovalModeTag:
			evt.MembershipApprovalMode = &types.GroupMembershipApprovalMode{
				IsJoinApprovalRequired: true,
			}
		case groupSuspendedTag:
			evt.Suspended = true
		case groupUnsuspendedTag:
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
	if len(children) == 1 && children[0].Tag == groupCreateTag {
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
