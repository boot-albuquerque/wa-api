package group

import (
	"wa-api/pkg/domain"
)

// Presenters for the group/community family, minus group-info (presenter.go).
//
// Hand-written, one function per type, field by field — see presenter.go for
// why: the property being bought is a COMPILE ERROR when a domain field is
// renamed, which a reflection-based copier would not give.

// PresentAcknowledgement builds the body of a write route that has nothing to
// report but that it happened.
func PresentAcknowledgement(details string) AcknowledgementResponse {
	return AcknowledgementResponse{Details: details}
}

// PresentListGroups maps the group listing.
func PresentListGroups(r *domain.ListGroupsResult) ListGroupsResponse {
	if r == nil {
		return ListGroupsResponse{Groups: []*GroupInfoResponse{}}
	}
	// Allocated even when empty: `[]` and `null` are different values to every
	// client, and only one of them can be ranged over without a check.
	out := ListGroupsResponse{Groups: make([]*GroupInfoResponse, 0, len(r.Groups))}
	for _, g := range r.Groups {
		out.Groups = append(out.Groups, PresentGroupInfo(g))
	}
	return out
}

// PresentGetGroupInviteLink maps the invite link.
func PresentGetGroupInviteLink(r *domain.GetGroupInviteLinkResult) GetGroupInviteLinkResponse {
	if r == nil {
		return GetGroupInviteLinkResponse{}
	}
	return GetGroupInviteLinkResponse{InviteLink: r.InviteLink}
}

// PresentGetGroupInviteInfo maps what a link says about a group.
func PresentGetGroupInviteInfo(r *domain.GetGroupInviteInfoResult) GetGroupInviteInfoResponse {
	if r == nil {
		return GetGroupInviteInfoResponse{}
	}
	return GetGroupInviteInfoResponse{InviteInfo: PresentGroupInfo(r.InviteInfo)}
}

// PresentCreatedGroup maps the outcome of asking for a group to exist.
func PresentCreatedGroup(r *domain.CreatedGroup) CreateGroupResponse {
	if r == nil {
		return CreateGroupResponse{}
	}
	return CreateGroupResponse{
		GroupInfo: PresentGroupInfo(r.Group),
		Created:   r.Created,
	}
}

// PresentParticipantsUpdate maps the outcome of adding or removing members.
//
// details is passed in rather than built here: the presenter's job is the
// SHAPE, and the sentence belongs to the route.
func PresentParticipantsUpdate(u domain.ParticipantsUpdate, details string) UpdateGroupParticipantsResponse {
	out := UpdateGroupParticipantsResponse{
		Details:      details,
		Participants: make([]GroupParticipantResponse, 0, len(u.Participants)),
		Confirmed:    u.Confirmed,
		Reason:       u.Reason,
	}
	for _, p := range u.Participants {
		out.Participants = append(out.Participants, PresentGroupParticipant(p))
	}
	return out
}

// PresentGroupJoinRequest maps one pending join request.
func PresentGroupJoinRequest(r domain.GroupJoinRequest) GroupJoinRequestResponse {
	return GroupJoinRequestResponse{
		JID:         string(r.JID),
		RequestedAt: presentTime(r.RequestedAt),
		AddedByJID:  string(r.AddedByJID),
		Method:      r.Method,
	}
}

// PresentGetGroupRequestParticipants maps the join-request queue.
func PresentGetGroupRequestParticipants(r *domain.GetGroupRequestParticipantsResult) GetGroupRequestParticipantsResponse {
	if r == nil {
		return GetGroupRequestParticipantsResponse{Requests: []GroupJoinRequestResponse{}}
	}
	out := GetGroupRequestParticipantsResponse{
		Requests: make([]GroupJoinRequestResponse, 0, len(r.Requests)),
	}
	for _, req := range r.Requests {
		out.Requests = append(out.Requests, PresentGroupJoinRequest(req))
	}
	return out
}

// PresentCommunitySubGroup maps one child group of a community.
func PresentCommunitySubGroup(g domain.CommunitySubGroup) CommunitySubGroupResponse {
	return CommunitySubGroupResponse{
		JID:               string(g.JID),
		Name:              g.Name,
		IsDefaultSubGroup: g.IsDefaultSubGroup,
	}
}

// PresentGetCommunitySubGroups maps a community's children.
func PresentGetCommunitySubGroups(r *domain.GetCommunitySubGroupsResult) GetCommunitySubGroupsResponse {
	if r == nil {
		return GetCommunitySubGroupsResponse{SubGroups: []CommunitySubGroupResponse{}}
	}
	out := GetCommunitySubGroupsResponse{
		SubGroups: make([]CommunitySubGroupResponse, 0, len(r.SubGroups)),
	}
	for _, g := range r.SubGroups {
		out.SubGroups = append(out.SubGroups, PresentCommunitySubGroup(g))
	}
	return out
}

// PresentGetCommunityParticipants maps who is in a community's linked groups.
func PresentGetCommunityParticipants(r *domain.GetCommunityParticipantsResult) GetCommunityParticipantsResponse {
	if r == nil {
		return GetCommunityParticipantsResponse{Participants: []string{}}
	}
	out := GetCommunityParticipantsResponse{
		Participants: make([]string, 0, len(r.Participants)),
	}
	for _, p := range r.Participants {
		out.Participants = append(out.Participants, string(p))
	}
	return out
}
