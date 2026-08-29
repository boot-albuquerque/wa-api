package group

import (
	"context"
	"fmt"

	waBinary "wa-api/internal/noise/protocol/binary"
	"wa-api/internal/noise/protocol/types"
)

// ParticipantChange e' a acao de UpdateParticipants. A raiz reexporta o tipo e
// as quatro constantes pelos nomes historicos.
type ParticipantChange string

const (
	ChangeAdd     ParticipantChange = "add"
	ChangeRemove  ParticipantChange = "remove"
	ChangePromote ParticipantChange = "promote"
	ChangeDemote  ParticipantChange = "demote"
)

// UpdateParticipants can be used to add, remove, promote and demote members in a WhatsApp group.
func UpdateParticipants(ctx context.Context, t Transport, jid types.JID, participantChanges []types.JID, action ParticipantChange) ([]types.GroupParticipant, error) {
	content := make([]waBinary.Node, len(participantChanges))
	for i, participantJID := range participantChanges {
		content[i] = waBinary.Node{
			Tag:   participantTag,
			Attrs: waBinary.Attrs{"jid": participantJID},
		}
		if participantJID.Server == types.HiddenUserServer && action == ChangeAdd {
			pn, err := t.Store().LIDs.GetPNForLID(ctx, participantJID)
			if err != nil {
				return nil, fmt.Errorf("failed to get phone number for LID %s: %v", participantJID, err)
			} else if !pn.IsEmpty() {
				content[i].Attrs["phone_number"] = pn
			}
		}
	}
	resp, err := sendIQ(ctx, t, IQSet, jid, waBinary.Node{
		Tag:     string(action),
		Content: content,
	})
	if err != nil {
		return nil, err
	}
	requestAction, ok := resp.GetOptionalChildByTag(string(action))
	if !ok {
		return nil, t.ElementMissing(string(action), "response to group participants update")
	}
	requestParticipants := requestAction.GetChildrenByTag(participantTag)
	participants := make([]types.GroupParticipant, len(requestParticipants))
	for i, child := range requestParticipants {
		participants[i] = ParseParticipant(child.AttrGetter(), &child)
	}
	return participants, nil
}

// GetRequestParticipants gets the list of participants that have requested to join the group.
func GetRequestParticipants(ctx context.Context, t Transport, jid types.JID) ([]types.GroupParticipantRequest, error) {
	resp, err := sendIQ(ctx, t, IQGet, jid, waBinary.Node{
		Tag: "membership_approval_requests",
	})
	if err != nil {
		return nil, err
	}
	request, ok := resp.GetOptionalChildByTag("membership_approval_requests")
	if !ok {
		return nil, t.ElementMissing("membership_approval_requests", "response to group request participants query")
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

// ParticipantRequestChange e' a acao de UpdateRequestParticipants.
type ParticipantRequestChange string

const (
	RequestApprove ParticipantRequestChange = "approve"
	RequestReject  ParticipantRequestChange = "reject"
)

// UpdateRequestParticipants can be used to approve or reject requests to join the group.
func UpdateRequestParticipants(ctx context.Context, t Transport, jid types.JID, participantChanges []types.JID, action ParticipantRequestChange) ([]types.GroupParticipant, error) {
	content := make([]waBinary.Node, len(participantChanges))
	for i, participantJID := range participantChanges {
		content[i] = waBinary.Node{
			Tag:   participantTag,
			Attrs: waBinary.Attrs{"jid": participantJID},
		}
	}
	resp, err := sendIQ(ctx, t, IQSet, jid, waBinary.Node{
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
		return nil, t.ElementMissing("membership_requests_action", "response to group request participants update")
	}
	requestAction, ok := request.GetOptionalChildByTag(string(action))
	if !ok {
		return nil, t.ElementMissing(string(action), "response to group request participants update")
	}
	requestParticipants := requestAction.GetChildrenByTag(participantTag)
	participants := make([]types.GroupParticipant, len(requestParticipants))
	for i, child := range requestParticipants {
		participants[i] = ParseParticipant(child.AttrGetter(), &child)
	}
	return participants, nil
}
