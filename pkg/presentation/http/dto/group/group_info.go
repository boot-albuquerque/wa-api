// Package group holds the PUBLIC wire types for the group family of routes,
// plus the hand-written presenters that build them from domain values.
//
// Nothing here may be reused by the domain or by a use case: these types exist
// to be serialized, and every field name in them is a promise to a client.
// See docs/HTTP-DTO-CONVENTIONS.md.
package group

// GroupParticipantResponse is one member of a group, as served.
type GroupParticipantResponse struct {
	JID          string `json:"jid"`
	PhoneNumber  string `json:"phone_number"`
	LID          string `json:"lid"`
	DisplayName  string `json:"display_name"`
	IsAdmin      bool   `json:"is_admin"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	// Error is non-zero when the protocol reports THIS participant's
	// operation failed while the rest of the batch succeeded — a partial
	// outcome on an add, or on a join-request approve/reject. Zero is also
	// what a plain roster read (not an update) always carries here.
	Error int `json:"error"`
}

// GroupInfoResponse is a group's metadata, as served.
//
// No `omitempty` anywhere. A client reading this has to be able to tell "the
// group has no topic" from "this key was dropped", and omitempty makes those
// two identical on the wire. Timestamps that the engine does not know are
// emitted as null (hence *string), which IS distinguishable from "" — the zero
// time formatted would read as year 1, a date that never happened.
type GroupInfoResponse struct {
	JID      string `json:"jid"`
	OwnerJID string `json:"owner_jid"`

	Name      string  `json:"name"`
	NameSetAt *string `json:"name_set_at"`
	NameSetBy string  `json:"name_set_by"`

	Topic      string  `json:"topic"`
	TopicSetAt *string `json:"topic_set_at"`
	TopicSetBy string  `json:"topic_set_by"`

	IsLocked          bool   `json:"is_locked"`
	IsAnnounce        bool   `json:"is_announce"`
	IsEphemeral       bool   `json:"is_ephemeral"`
	DisappearingTimer uint32 `json:"disappearing_timer"`
	IsIncognito       bool   `json:"is_incognito"`
	IsSuspended       bool   `json:"is_suspended"`

	IsParent               bool   `json:"is_parent"`
	LinkedParentJID        string `json:"linked_parent_jid"`
	IsDefaultSubGroup      bool   `json:"is_default_sub_group"`
	IsJoinApprovalRequired bool   `json:"is_join_approval_required"`

	MemberAddMode string  `json:"member_add_mode"`
	CreatedAt     *string `json:"created_at"`

	ParticipantCount int                        `json:"participant_count"`
	Participants     []GroupParticipantResponse `json:"participants"`
}

// GetGroupInfoResponse is the body of the `data` key for the group-info route.
//
// It wraps instead of inlining because the route already answered
// `{"group_info": {...}}` and the wrapper is where a future `meta` would go
// without moving every existing field.
type GetGroupInfoResponse struct {
	GroupInfo *GroupInfoResponse `json:"group_info"`
}
