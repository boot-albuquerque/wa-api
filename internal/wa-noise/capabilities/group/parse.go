package group

import (
	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// ParseParticipant le um no <participant>.
func ParseParticipant(childAG *waBinary.AttrUtility, child *waBinary.Node) types.GroupParticipant {
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
		addRequest, ok := child.GetOptionalChildByTag(addRequestTag)
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

// ParseNode le um no <group> completo em types.GroupInfo.
func ParseNode(t Transport, groupNode *waBinary.Node) (*types.GroupInfo, error) {
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
		case participantTag:
			group.Participants = append(group.Participants, ParseParticipant(childAG, &child))
		case descriptionTag:
			body, bodyOK := child.GetOptionalChildByTag(descriptionBodyTag)
			if bodyOK {
				topicBytes, _ := body.Content.([]byte)
				group.Topic = string(topicBytes)
				group.TopicID = childAG.String("id")
				group.TopicSetBy = childAG.OptionalJIDOrEmpty("participant")
				group.TopicSetByPN = childAG.OptionalJIDOrEmpty("participant_pn") // TODO confirm field name
				group.TopicSetAt = childAG.UnixTime("t")
			}
		case announcementTag:
			group.IsAnnounce = true
		case lockedTag:
			group.IsLocked = true
		case ephemeralTag:
			group.IsEphemeral = true
			group.DisappearingTimer = uint32(childAG.Uint64("expiration"))
		case memberAddModeTag:
			modeBytes, _ := child.Content.([]byte)
			group.MemberAddMode = types.GroupMemberAddMode(modeBytes)
		case linkedParentTag:
			group.LinkedParentJID = childAG.JID("jid")
		case defaultSubGroupTag:
			group.IsDefaultSubGroup = true
		case parentTag:
			group.IsParent = true
			group.DefaultMembershipApprovalMode = childAG.OptionalString("default_membership_approval_mode")
		case incognitoTag:
			group.IsIncognito = true
		case membershipApprovalModeTag:
			group.IsJoinApprovalRequired = true
		case suspendedTag:
			group.Suspended = true
		default:
			t.Log().Debugf("Unknown element in group node %s: %s", group.JID.String(), child.XMLString())
		}
		if !childAG.OK() {
			t.Log().Warnf("Possibly failed to parse %s element in group node: %+v", child.Tag, childAG.Errors)
		}
	}

	return &group, ag.Error()
}

// ParseLinkTargetNode le o <group> reduzido que aparece em <link>/<unlink> e na
// lista de subgrupos.
func ParseLinkTargetNode(groupNode *waBinary.Node) (types.GroupLinkTarget, error) {
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
			IsDefaultSubGroup: groupNode.GetChildByTag(defaultSubGroupTag).Tag == defaultSubGroupTag,
		},
	}, ag.Error()
}

// ParseParticipantList le os <participant> filhos de node, devolvendo os JIDs e
// os pares LID/PN que der para deduzir dos atributos.
func ParseParticipantList(node *waBinary.Node) (participants []types.JID, lidPairs []store.LIDMapping) {
	children := node.GetChildren()
	participants = make([]types.JID, 0, len(children))
	for _, child := range children {
		jid, ok := child.Attrs["jid"].(types.JID)
		if child.Tag != participantTag || !ok {
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
