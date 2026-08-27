package group

import (
	"time"

	"wa-api/pkg/domain"
)

// Presenters are hand-written, one function per type, field by field.
//
// No reflection, no generic struct copier, no `json` round-trip. The property
// being bought is a COMPILE ERROR: rename a field in domain.GroupInfo and this
// file stops building. A reflection-based mapper would keep building and would
// silently drop the key from every response, which is the exact failure the
// DTO layer exists to prevent.

// PresentGroupParticipant maps one participant.
func PresentGroupParticipant(p domain.GroupParticipant) GroupParticipantResponse {
	return GroupParticipantResponse{
		JID:          string(p.JID),
		PhoneNumber:  string(p.PhoneNumber),
		LID:          string(p.LID),
		DisplayName:  p.DisplayName,
		IsAdmin:      p.IsAdmin,
		IsSuperAdmin: p.IsSuperAdmin,
	}
}

// PresentGroupInfo maps a group's metadata. A nil input presents as nil, so
// the caller does not have to branch before calling.
func PresentGroupInfo(g *domain.GroupInfo) *GroupInfoResponse {
	if g == nil {
		return nil
	}
	out := &GroupInfoResponse{
		JID:      string(g.JID),
		OwnerJID: string(g.OwnerJID),

		Name:      g.Name,
		NameSetAt: presentTime(g.NameSetAt),
		NameSetBy: string(g.NameSetBy),

		Topic:      g.Topic,
		TopicSetAt: presentTime(g.TopicSetAt),
		TopicSetBy: string(g.TopicSetBy),

		IsLocked:          g.IsLocked,
		IsAnnounce:        g.IsAnnounce,
		IsEphemeral:       g.IsEphemeral,
		DisappearingTimer: g.DisappearingTimer,
		IsIncognito:       g.IsIncognito,
		IsSuspended:       g.IsSuspended,

		IsParent:               g.IsParent,
		LinkedParentJID:        string(g.LinkedParentJID),
		IsDefaultSubGroup:      g.IsDefaultSubGroup,
		IsJoinApprovalRequired: g.IsJoinApprovalRequired,

		MemberAddMode: g.MemberAddMode,
		CreatedAt:     presentTime(g.CreatedAt),

		ParticipantCount: g.ParticipantCount,
	}
	// Allocated even for an empty roster: `[]` and `null` are different values
	// to every client, and only one of them can be ranged over without a check.
	out.Participants = make([]GroupParticipantResponse, 0, len(g.Participants))
	for _, p := range g.Participants {
		out.Participants = append(out.Participants, PresentGroupParticipant(p))
	}
	return out
}

// PresentGetGroupInfo maps the use case result onto the route's `data`.
func PresentGetGroupInfo(r *domain.GetGroupInfoResult) GetGroupInfoResponse {
	if r == nil {
		return GetGroupInfoResponse{}
	}
	return GetGroupInfoResponse{GroupInfo: PresentGroupInfo(r.GroupInfo)}
}

// presentTime renders a timestamp as RFC 3339 in UTC, or null when the domain
// has no value for it.
//
// The zero time is null and not "0001-01-01T00:00:00Z": that string is a real
// date on the wire, and a client that parses it gets a group created before
// the Gregorian calendar instead of a missing field.
func presentTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
