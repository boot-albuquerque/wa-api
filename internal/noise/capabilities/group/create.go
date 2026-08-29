package group

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// ReqCreate contains the request data for Create. A raiz reexporta este tipo
// como wa-noise.ReqCreateGroup (apelido, mesmo tipo).
type ReqCreate struct {
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

// Create creates a group on WhatsApp with the given name and participants.
//
// See ReqCreate for parameters.
func Create(ctx context.Context, t Transport, req ReqCreate) (*types.GroupInfo, error) {
	participantNodes := make([]waBinary.Node, len(req.Participants), len(req.Participants)+1)
	for i, participant := range req.Participants {
		participantNodes[i] = waBinary.Node{
			Tag:   participantTag,
			Attrs: waBinary.Attrs{"jid": participant},
		}
		pt, err := t.Store().PrivacyTokens.GetPrivacyToken(ctx, participant)
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
		req.CreateKey = t.GenerateMessageID()
	}
	if req.IsParent {
		if req.DefaultMembershipApprovalMode == "" {
			req.DefaultMembershipApprovalMode = defaultMembershipApprovalMode
		}
		participantNodes = append(participantNodes, waBinary.Node{
			Tag: parentTag,
			Attrs: waBinary.Attrs{
				"default_membership_approval_mode": req.DefaultMembershipApprovalMode,
			},
		})
	} else if !req.LinkedParentJID.IsEmpty() {
		participantNodes = append(participantNodes, waBinary.Node{
			Tag:   linkedParentTag,
			Attrs: waBinary.Attrs{"jid": req.LinkedParentJID},
		})
	}
	if req.IsLocked {
		participantNodes = append(participantNodes, waBinary.Node{Tag: lockedTag})
	}
	if req.IsAnnounce {
		participantNodes = append(participantNodes, waBinary.Node{Tag: announcementTag})
	}
	if req.IsEphemeral {
		participantNodes = append(participantNodes, waBinary.Node{
			Tag: ephemeralTag,
			Attrs: waBinary.Attrs{
				"expiration": req.DisappearingTimer,
				"trigger":    "1", // TODO what's this?
			},
		})
	}
	if req.IsJoinApprovalRequired {
		participantNodes = append(participantNodes, waBinary.Node{
			Tag: membershipApprovalModeTag,
			Content: []waBinary.Node{{
				Tag:   joinTag,
				Attrs: waBinary.Attrs{"state": joinStateOn},
			}},
		})
	}
	// WhatsApp web doesn't seem to include the static prefix for these
	key := t.TrimMessageIDPrefix(req.CreateKey)
	resp, err := sendIQ(ctx, t, IQSet, types.GroupServerJID, waBinary.Node{
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
	groupNode, ok := resp.GetOptionalChildByTag(nodeTag)
	if !ok {
		return nil, t.ElementMissing(nodeTag, "response to create group query")
	}
	return ParseNode(t, &groupNode)
}

// Unlink removes a child group from a parent community.
func Unlink(ctx context.Context, t Transport, parent, child types.JID) error {
	_, err := sendIQ(ctx, t, IQSet, parent, waBinary.Node{
		Tag:   unlinkTag,
		Attrs: waBinary.Attrs{"unlink_type": string(types.GroupLinkChangeTypeSub)},
		Content: []waBinary.Node{{
			Tag:   nodeTag,
			Attrs: waBinary.Attrs{"jid": child},
		}},
	})
	return err
}

// Link adds an existing group as a child group in a community.
//
// To create a new group within a community, set LinkedParentJID in the Create request.
func Link(ctx context.Context, t Transport, parent, child types.JID) error {
	_, err := sendIQ(ctx, t, IQSet, parent, waBinary.Node{
		Tag: "links",
		Content: []waBinary.Node{{
			Tag:   linkTag,
			Attrs: waBinary.Attrs{"link_type": string(types.GroupLinkChangeTypeSub)},
			Content: []waBinary.Node{{
				Tag:   nodeTag,
				Attrs: waBinary.Attrs{"jid": child},
			}},
		}},
	})
	return err
}

// Leave leaves the specified group on WhatsApp.
func Leave(ctx context.Context, t Transport, jid types.JID) error {
	_, err := sendIQ(ctx, t, IQSet, types.GroupServerJID, waBinary.Node{
		Tag: "leave",
		Content: []waBinary.Node{{
			Tag:   nodeTag,
			Attrs: waBinary.Attrs{"id": jid},
		}},
	})
	return err
}
