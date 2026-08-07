// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package group

import (
	"fmt"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// ParseCreate le a notificacao de criacao de grupo.
func ParseCreate(t Transport, parentNode, node *waBinary.Node) (*events.JoinedGroup, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	groupNode, ok := node.GetOptionalChildByTag(nodeTag)
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
	info, err := ParseNode(t, &groupNode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse group info in create notification: %w", err)
	}
	evt.GroupInfo = *info
	lidPairs, redactedPhones := CacheInfo(t, info, true)
	return &evt, lidPairs, redactedPhones, nil
}

// collectParticipantList le a lista de participantes de child e **acumula** os
// pares LID/PN encontrados em lidPairs, em vez de substituir o que ja' estava
// la'. Um mesmo <notification type="w:gp2"> pode trazer mais de um elemento de
// participante (por exemplo <add> e <remove> na mesma notificacao), e os pares
// de todos eles precisam chegar ao PutManyLIDMappings do chamador.
//
// Correcao da Fase E lote 6, preservada palavra por palavra nesta extracao.
func collectParticipantList(child *waBinary.Node, lidPairs *[]store.LIDMapping) []types.JID {
	participants, childPairs := ParseParticipantList(child)
	*lidPairs = append(*lidPairs, childPairs...)
	return participants
}

// ParseChange le a notificacao de mudanca de grupo.
func ParseChange(t Transport, node *waBinary.Node) (*events.GroupInfo, []store.LIDMapping, error) {
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
		case ChangeAdd, ChangeRemove, ChangePromote, ChangeDemote:
			evt.PrevParticipantVersionID = cag.OptionalString("prev_v_id")
			evt.ParticipantVersionID = cag.OptionalString("v_id")
		}
		switch child.Tag {
		case string(ChangeAdd):
			evt.JoinReason = cag.OptionalString("reason")
			evt.Join = collectParticipantList(&child, &lidPairs)
		case string(ChangeRemove):
			evt.Leave = collectParticipantList(&child, &lidPairs)
		case string(ChangePromote):
			evt.Promote = collectParticipantList(&child, &lidPairs)
		case string(ChangeDemote):
			evt.Demote = collectParticipantList(&child, &lidPairs)
		case lockedTag:
			evt.Locked = &types.GroupLocked{IsLocked: true}
		case unlockedTag:
			evt.Locked = &types.GroupLocked{IsLocked: false}
		case deleteTag:
			evt.Delete = &types.GroupDelete{Deleted: true, DeleteReason: cag.String("reason")}
		case subjectTag:
			evt.Name = &types.GroupName{
				Name:        cag.String("subject"),
				NameSetAt:   cag.UnixTime("s_t"),
				NameSetBy:   cag.OptionalJIDOrEmpty("s_o"),
				NameSetByPN: cag.OptionalJIDOrEmpty("s_o_pn"),
			}
		case descriptionTag:
			var topicStr string
			_, isDelete := child.GetOptionalChildByTag(deleteTag)
			if !isDelete {
				topicChild := child.GetChildByTag(descriptionBodyTag)
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
		case announcementTag:
			evt.Announce = &types.GroupAnnounce{
				IsAnnounce:        true,
				AnnounceVersionID: cag.String("v_id"),
			}
		case notAnnouncementTag:
			evt.Announce = &types.GroupAnnounce{
				IsAnnounce:        false,
				AnnounceVersionID: cag.String("v_id"),
			}
		case inviteTag:
			link := InviteLinkPrefix + cag.String("code")
			evt.NewInviteLink = &link
		case ephemeralTag:
			timer := uint32(cag.Uint64("expiration"))
			evt.Ephemeral = &types.GroupEphemeral{
				IsEphemeral:       true,
				DisappearingTimer: timer,
			}
		case notEphemeralTag:
			evt.Ephemeral = &types.GroupEphemeral{IsEphemeral: false}
		case linkTag:
			evt.Link = &types.GroupLinkChange{
				Type: types.GroupLinkChangeType(cag.String("link_type")),
			}
			groupNode, ok := child.GetOptionalChildByTag(nodeTag)
			if !ok {
				return nil, nil, t.ElementMissing(nodeTag, "group link")
			}
			var err error
			evt.Link.Group, err = ParseLinkTargetNode(&groupNode)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse group link node in group change: %w", err)
			}
		case unlinkTag:
			evt.Unlink = &types.GroupLinkChange{
				Type:         types.GroupLinkChangeType(cag.String("unlink_type")),
				UnlinkReason: types.GroupUnlinkReason(cag.String("unlink_reason")),
			}
			groupNode, ok := child.GetOptionalChildByTag(nodeTag)
			if !ok {
				return nil, nil, t.ElementMissing(nodeTag, "group unlink")
			}
			var err error
			evt.Unlink.Group, err = ParseLinkTargetNode(&groupNode)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse group unlink node in group change: %w", err)
			}
		case membershipApprovalModeTag:
			evt.MembershipApprovalMode = &types.GroupMembershipApprovalMode{
				IsJoinApprovalRequired: true,
			}
		case suspendedTag:
			evt.Suspended = true
		case unsuspendedTag:
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

// UpdateParticipantCache aplica as entradas e saidas de evt a' entrada em cache
// do grupo, se ela existir.
//
// O lock e' tomado aqui e segurado pelo read-modify-write inteiro, exatamente
// como no updateGroupParticipantCache da raiz: o *Meta devolvido por GetLocked
// e' um ponteiro para a entrada viva do mapa e cached.Members e' mutado no
// lugar, entao soltar o lock entre a leitura e a escrita perderia atualizacoes.
func UpdateParticipantCache(t Transport, evt *events.GroupInfo) {
	// TODO can the addressing mode change here?
	if len(evt.Join) == 0 && len(evt.Leave) == 0 {
		return
	}
	cache := t.Cache()
	cache.Lock()
	defer cache.Unlock()
	cached, ok := cache.GetLocked(evt.JID)
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

// ParseNotification roteia um <notification type="w:gp2"> entre criacao de
// grupo e mudanca de grupo.
func ParseNotification(t Transport, node *waBinary.Node) (any, []store.LIDMapping, []store.RedactedPhoneEntry, error) {
	children := node.GetChildren()
	if len(children) == 1 && children[0].Tag == createTag {
		return ParseCreate(t, node, &children[0])
	} else {
		change, lidPairs, err := ParseChange(t, node)
		if err != nil {
			return nil, nil, nil, err
		}
		UpdateParticipantCache(t, change)
		return change, lidPairs, nil, nil
	}
}
