// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/waclient/binary"
	"wa-api/internal/waclient/types"
)

type ParticipantChange string

const (
	ParticipantChangeAdd     ParticipantChange = "add"
	ParticipantChangeRemove  ParticipantChange = "remove"
	ParticipantChangePromote ParticipantChange = "promote"
	ParticipantChangeDemote  ParticipantChange = "demote"
)

// UpdateGroupParticipants can be used to add, remove, promote and demote members in a WhatsApp group.
func (cli *Client) UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action ParticipantChange) ([]types.GroupParticipant, error) {
	content := make([]waBinary.Node, len(participantChanges))
	for i, participantJID := range participantChanges {
		content[i] = waBinary.Node{
			Tag:   "participant",
			Attrs: waBinary.Attrs{"jid": participantJID},
		}
		if participantJID.Server == types.HiddenUserServer && action == ParticipantChangeAdd {
			pn, err := cli.Store.LIDs.GetPNForLID(ctx, participantJID)
			if err != nil {
				return nil, fmt.Errorf("failed to get phone number for LID %s: %v", participantJID, err)
			} else if !pn.IsEmpty() {
				content[i].Attrs["phone_number"] = pn
			}
		}
	}
	resp, err := cli.sendGroupIQ(ctx, iqSet, jid, waBinary.Node{
		Tag:     string(action),
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	requestAction, ok := resp.GetOptionalChildByTag(string(action))
	if !ok {
		return nil, &ElementMissingError{Tag: string(action), In: "response to group participants update"}
	}
	requestParticipants := requestAction.GetChildrenByTag("participant")
	participants := make([]types.GroupParticipant, len(requestParticipants))
	for i, child := range requestParticipants {
		participants[i] = parseParticipant(child.AttrGetter(), &child)
	}
	return participants, nil
}

// GetGroupRequestParticipants gets the list of participants that have requested to join the group.
func (cli *Client) GetGroupRequestParticipants(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
	resp, err := cli.sendGroupIQ(ctx, iqGet, jid, waBinary.Node{
		Tag: "membership_approval_requests",
	})
	if err != nil {
		return nil, err
	}
	request, ok := resp.GetOptionalChildByTag("membership_approval_requests")
	if !ok {
		return nil, &ElementMissingError{Tag: "membership_approval_requests", In: "response to group request participants query"}
	}
	requestParticipants := request.GetChildrenByTag("membership_approval_request")
	participants := make([]types.GroupParticipantRequest, len(requestParticipants))
	for i, req := range requestParticipants {
		participants[i] = types.GroupParticipantRequest{
			JID:         req.AttrGetter().JID("jid"),
			RequestedAt: req.AttrGetter().UnixTime("request_time"),
		}
	}
	return participants, nil
}

type ParticipantRequestChange string

const (
	ParticipantChangeApprove ParticipantRequestChange = "approve"
	ParticipantChangeReject  ParticipantRequestChange = "reject"
)

// UpdateGroupRequestParticipants can be used to approve or reject requests to join the group.
func (cli *Client) UpdateGroupRequestParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action ParticipantRequestChange) ([]types.GroupParticipant, error) {
	content := make([]waBinary.Node, len(participantChanges))
	for i, participantJID := range participantChanges {
		content[i] = waBinary.Node{
			Tag:   "participant",
			Attrs: waBinary.Attrs{"jid": participantJID},
		}
	}
	resp, err := cli.sendGroupIQ(ctx, iqSet, jid, waBinary.Node{
		Tag: "membership_requests_action",
		Content: []waBinary.Node{{
			Tag:     string(action),
			Content: content,
		}},
	})
	if err != nil {
		return nil, err
	}
	request, ok := resp.GetOptionalChildByTag("membership_requests_action")
	if !ok {
		return nil, &ElementMissingError{Tag: "membership_requests_action", In: "response to group request participants update"}
	}
	requestAction, ok := request.GetOptionalChildByTag(string(action))
	if !ok {
		return nil, &ElementMissingError{Tag: string(action), In: "response to group request participants update"}
	}
	requestParticipants := requestAction.GetChildrenByTag("participant")
	participants := make([]types.GroupParticipant, len(requestParticipants))
	for i, child := range requestParticipants {
		participants[i] = parseParticipant(child.AttrGetter(), &child)
	}
	return participants, nil
}
