package domain

import "time"

// GroupInfo is the engine-independent view of a WhatsApp group's metadata.
//
// It exists because appport.GroupDirectory.GetGroupInfo used to return `any`,
// and the two engines put DIFFERENT concrete types in it: the wa-noise
// protocol struct (internal/wa-noise/protocol/types.GroupInfo) and the
// headless conversation struct (internal/wa-headless/capabilities/chats.Chat).
// With `any`, whichever engine served the session decided the JSON the client
// received, and neither shape was ever declared anywhere. A named domain type
// makes the union explicit and gives the presenter something it can map field
// by field.
//
// Field names are Go-idiomatic on purpose: this type is NOT the wire format.
// The wire format is dto/group.GroupInfoResponse, and the mapping between them
// is hand-written (see pkg/presentation/http/dto/group/presenter.go).
//
// What the headless engine cannot observe stays at its zero value. The
// presenter still emits those keys, so a client never has to tell "absent"
// from "served by the other engine".
type GroupInfo struct {
	JID      JID
	OwnerJID JID

	Name      string
	NameSetAt time.Time
	NameSetBy JID

	Topic      string
	TopicSetAt time.Time
	TopicSetBy JID

	IsLocked          bool
	IsAnnounce        bool
	IsEphemeral       bool
	DisappearingTimer uint32
	IsIncognito       bool
	IsSuspended       bool

	IsParent               bool
	LinkedParentJID        JID
	IsDefaultSubGroup      bool
	IsJoinApprovalRequired bool

	// MemberAddMode is "admin_add" or "all_member_add" upstream. Kept as a
	// string rather than a named type because the domain does not branch on
	// it — it only carries it to the caller.
	MemberAddMode string

	CreatedAt time.Time

	// ParticipantCount is what the server reported, which is NOT always
	// len(Participants): a non-admin reading an announcement group gets the
	// count without the roster.
	ParticipantCount int
	Participants     []GroupParticipant
}

// GroupParticipant is one member of a group, engine-independent.
type GroupParticipant struct {
	// JID is the identity to address this participant by — the LID or the
	// phone number, whichever the server chose for this group.
	JID         JID
	PhoneNumber JID
	LID         JID

	// DisplayName is only present for anonymous participants in announcement
	// groups, where it stands in for an obfuscated phone number.
	DisplayName string

	IsAdmin      bool
	IsSuperAdmin bool
}
