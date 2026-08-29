package group

import (
	"wa-api/internal/noise/protocol/types"
	"wa-api/pkg/domain"
)

// toDomainGroupInfo maps the wa-noise protocol struct onto the domain type.
//
// Hand-written on purpose. A reflection-based copy would keep compiling after
// an upstream rename and silently drop the field from every response; this
// version stops compiling.
func toDomainGroupInfo(src *types.GroupInfo) *domain.GroupInfo {
	if src == nil {
		return nil
	}
	out := &domain.GroupInfo{
		JID:      domain.JID(src.JID.String()),
		OwnerJID: jidOrEmpty(src.OwnerJID),

		Name:      src.Name,
		NameSetAt: src.NameSetAt,
		NameSetBy: jidOrEmpty(src.NameSetBy),

		Topic:      src.Topic,
		TopicSetAt: src.TopicSetAt,
		TopicSetBy: jidOrEmpty(src.TopicSetBy),

		IsLocked:          src.IsLocked,
		IsAnnounce:        src.IsAnnounce,
		IsEphemeral:       src.IsEphemeral,
		DisappearingTimer: src.DisappearingTimer,
		IsIncognito:       src.IsIncognito,
		IsSuspended:       src.Suspended,

		IsParent:               src.IsParent,
		LinkedParentJID:        jidOrEmpty(src.LinkedParentJID),
		IsDefaultSubGroup:      src.IsDefaultSubGroup,
		IsJoinApprovalRequired: src.IsJoinApprovalRequired,

		MemberAddMode: string(src.MemberAddMode),
		CreatedAt:     src.GroupCreated,

		ParticipantCount: src.ParticipantCount,
	}
	// Non-nil even when empty: an empty roster and an absent roster are the
	// same thing to a caller reading `participants`, and `null` there makes
	// every client add a nil check that buys nothing.
	out.Participants = make([]domain.GroupParticipant, 0, len(src.Participants))
	for _, p := range src.Participants {
		out.Participants = append(out.Participants, toDomainGroupParticipant(p))
	}
	return out
}

// toDomainGroupParticipant maps one member. Shared with the participants
// adapter, which reads the same protocol struct back from an add/remove call.
func toDomainGroupParticipant(p types.GroupParticipant) domain.GroupParticipant {
	return domain.GroupParticipant{
		JID:          jidOrEmpty(p.JID),
		PhoneNumber:  jidOrEmpty(p.PhoneNumber),
		LID:          jidOrEmpty(p.LID),
		DisplayName:  p.DisplayName,
		IsAdmin:      p.IsAdmin,
		IsSuperAdmin: p.IsSuperAdmin,
		Error:        p.Error,
	}
}

// jidOrEmpty renders a JID, mapping the zero JID to "" instead of to the
// literal "@" that types.JID.String() produces for it.
func jidOrEmpty(jid types.JID) domain.JID {
	if jid.IsEmpty() {
		return ""
	}
	return domain.JID(jid.String())
}
