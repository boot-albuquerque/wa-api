// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

func parseParticipant(childAG *waBinary.AttrUtility, child *waBinary.Node) types.GroupParticipant {
	pcpType := childAG.OptionalString("type")
	participant := types.GroupParticipant{
		IsAdmin:      pcpType == participantTypeAdmin || pcpType == participantTypeSuperAdmin,
		IsSuperAdmin: pcpType == participantTypeSuperAdmin,
		JID:          childAG.JID("jid"),
		DisplayName:  childAG.OptionalString("display_name"),
	}
	if participant.JID.Server == types.HiddenUserServer {
		participant.LID = participant.JID
		participant.PhoneNumber = childAG.OptionalJIDOrEmpty("phone_number")
	} else if participant.JID.Server == types.DefaultUserServer {
		participant.PhoneNumber = participant.JID
		participant.LID = childAG.OptionalJIDOrEmpty("lid")
	}
	if errorCode := childAG.OptionalInt("error"); errorCode != 0 {
		participant.Error = errorCode
		addRequest, ok := child.GetOptionalChildByTag(groupAddRequestTag)
		if ok {
			addAG := addRequest.AttrGetter()
			participant.AddRequest = &types.GroupParticipantAddRequest{
				Code:       addAG.String("code"),
				Expiration: addAG.UnixTime("expiration"),
			}
		}
	}
	return participant
}

func (cli *Client) parseGroupNode(groupNode *waBinary.Node) (*types.GroupInfo, error) {
	var group types.GroupInfo
	ag := groupNode.AttrGetter()

	group.JID = types.NewJID(ag.String("id"), types.GroupServer)
	group.OwnerJID = ag.OptionalJIDOrEmpty("creator")
	group.OwnerPN = ag.OptionalJIDOrEmpty("creator_pn")

	group.Name = ag.OptionalString("subject")
	group.NameSetAt = ag.OptionalUnixTime("s_t")
	group.NameSetBy = ag.OptionalJIDOrEmpty("s_o")
	group.NameSetByPN = ag.OptionalJIDOrEmpty("s_o_pn")

	group.GroupCreated = ag.UnixTime("creation")
	group.CreatorCountryCode = ag.OptionalString("creator_country_code")

	group.AnnounceVersionID = ag.OptionalString("a_v_id")
	group.ParticipantVersionID = ag.OptionalString("p_v_id")
	group.ParticipantCount = ag.OptionalInt("size")
	group.AddressingMode = types.AddressingMode(ag.OptionalString("addressing_mode"))

	for _, child := range groupNode.GetChildren() {
		childAG := child.AttrGetter()
		switch child.Tag {
		case groupParticipantTag:
			group.Participants = append(group.Participants, parseParticipant(childAG, &child))
		case groupDescriptionTag:
			body, bodyOK := child.GetOptionalChildByTag(groupDescriptionBodyTag)
			if bodyOK {
				topicBytes, _ := body.Content.([]byte)
				group.Topic = string(topicBytes)
				group.TopicID = childAG.String("id")
				group.TopicSetBy = childAG.OptionalJIDOrEmpty("participant")
				group.TopicSetByPN = childAG.OptionalJIDOrEmpty("participant_pn") // TODO confirm field name
				group.TopicSetAt = childAG.UnixTime("t")
			}
		case groupAnnouncementTag:
			group.IsAnnounce = true
		case groupLockedTag:
			group.IsLocked = true
		case groupEphemeralTag:
			group.IsEphemeral = true
			group.DisappearingTimer = uint32(childAG.Uint64("expiration"))
		case groupMemberAddModeTag:
			modeBytes, _ := child.Content.([]byte)
			group.MemberAddMode = types.GroupMemberAddMode(modeBytes)
		case groupLinkedParentTag:
			group.LinkedParentJID = childAG.JID("jid")
		case groupDefaultSubGroupTag:
			group.IsDefaultSubGroup = true
		case groupParentTag:
			group.IsParent = true
			group.DefaultMembershipApprovalMode = childAG.OptionalString("default_membership_approval_mode")
		case groupIncognitoTag:
			group.IsIncognito = true
		case groupMembershipApprovalModeTag:
			group.IsJoinApprovalRequired = true
		case groupSuspendedTag:
			group.Suspended = true
		default:
			cli.Log.Debugf("Unknown element in group node %s: %s", group.JID.String(), child.XMLString())
		}
		if !childAG.OK() {
			cli.Log.Warnf("Possibly failed to parse %s element in group node: %+v", child.Tag, childAG.Errors)
		}
	}

	return &group, ag.Error()
}

func parseGroupLinkTargetNode(groupNode *waBinary.Node) (types.GroupLinkTarget, error) {
	ag := groupNode.AttrGetter()
	jidKey := ag.OptionalJIDOrEmpty("jid")
	if jidKey.IsEmpty() {
		jidKey = types.NewJID(ag.String("id"), types.GroupServer)
	}
	return types.GroupLinkTarget{
		JID: jidKey,
		GroupName: types.GroupName{
			Name:      ag.OptionalString("subject"),
			NameSetAt: ag.OptionalUnixTime("s_t"),
		},
		GroupIsDefaultSub: types.GroupIsDefaultSub{
			IsDefaultSubGroup: groupNode.GetChildByTag(groupDefaultSubGroupTag).Tag == groupDefaultSubGroupTag,
		},
	}, ag.Error()
}

func parseParticipantList(node *waBinary.Node) (participants []types.JID, lidPairs []store.LIDMapping) {
	children := node.GetChildren()
	participants = make([]types.JID, 0, len(children))
	for _, child := range children {
		jid, ok := child.Attrs["jid"].(types.JID)
		if child.Tag != groupParticipantTag || !ok {
			continue
		}
		participants = append(participants, jid)
		if jid.Server == types.HiddenUserServer {
			phoneNumber, ok := child.Attrs["phone_number"].(types.JID)
			if ok && !phoneNumber.IsEmpty() {
				lidPairs = append(lidPairs, store.LIDMapping{
					LID: jid,
					PN:  phoneNumber,
				})
			}
		} else if jid.Server == types.DefaultUserServer {
			lid, ok := child.Attrs["lid"].(types.JID)
			if ok && !lid.IsEmpty() {
				lidPairs = append(lidPairs, store.LIDMapping{
					LID: lid,
					PN:  jid,
				})
			}
		}
	}
	return
}
