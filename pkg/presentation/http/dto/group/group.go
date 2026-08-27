package group

// The wire types of the group and community family, minus the group-info route
// (that one lives in group_info.go, which is the reference implementation).
//
// No `omitempty` anywhere, on purpose: a client has to be able to tell "the
// group has no topic" from "this key was dropped in this version", and
// omitempty makes those identical on the wire. See
// docs/HTTP-DTO-CONVENTIONS.md §6.

// AcknowledgementResponse is the body of `data` for every group write that has
// nothing to report but that it happened: leave, name, topic, photo, announce,
// locked, ephemeral, join-approval mode, community link and unlink.
//
// One type for all of them instead of nine identical ones: they carry the same
// single field, and nine names would be nine places to change when the shape
// grows. The MESSAGE differs per route, and that is a value, not a type.
type AcknowledgementResponse struct {
	Details string `json:"details"`
}

// ListGroupsResponse is the body of `data` for the group listing.
//
// It wraps instead of serving a bare array so a future `meta` (paging) has a
// place to go without moving every existing field. No route paginates today —
// see docs/HTTP-DTO-CONVENTIONS.md §7.
type ListGroupsResponse struct {
	Groups []*GroupInfoResponse `json:"groups"`
}

// GetGroupInviteLinkResponse is the body of `data` for the invite-link route.
type GetGroupInviteLinkResponse struct {
	InviteLink string `json:"invite_link"`
}

// GetGroupInviteInfoResponse is the body of `data` for the invite-info route.
//
// The payload is a group's metadata, so it reuses GroupInfoResponse rather than
// declaring a second, near-identical shape a client would have to learn twice.
type GetGroupInviteInfoResponse struct {
	InviteInfo *GroupInfoResponse `json:"invite_info"`
}

// CreateGroupResponse is the body of `data` for the create-group route.
type CreateGroupResponse struct {
	GroupInfo *GroupInfoResponse `json:"group_info"`
	// Created is false when the group was found instead of made. Only the
	// headless engine can answer false — its capability is "ensure this group
	// exists" — and a client that cannot tell the two apart cannot know whether
	// it just changed someone's account.
	Created bool `json:"created"`
}

// UpdateGroupParticipantsResponse is the body of `data` for the
// participants-update route.
type UpdateGroupParticipantsResponse struct {
	Details string `json:"details"`
	// Participants is the roster the transport read back. Empty — and never
	// null — when the engine reads none, which is the normal case on the
	// headless engine; `confirmed` is where that is said.
	Participants []GroupParticipantResponse `json:"participants"`
	// Confirmed says the session that made the change READ IT BACK. False is
	// not a failure: the operation was sent and accepted, and nobody verified
	// it. Reason says why not, and is empty exactly when confirmed is true.
	Confirmed bool   `json:"confirmed"`
	Reason    string `json:"reason"`
}

// GroupJoinRequestResponse is one pending request to join a group.
type GroupJoinRequestResponse struct {
	JID string `json:"jid"`
	// RequestedAt is null, and never "0001-01-01T00:00:00Z", when the engine
	// does not report a moment. That string is a real date on the wire.
	RequestedAt *string `json:"requested_at"`
	// AddedByJID is empty when nobody added them — a request made by following
	// an invite link has no adder.
	AddedByJID string `json:"added_by_jid"`
	// Method is how the request arrived ("InviteLink" for a link). Empty when
	// the engine does not report it.
	Method string `json:"method"`
}

// GetGroupRequestParticipantsResponse is the body of `data` for the
// join-requests listing.
type GetGroupRequestParticipantsResponse struct {
	Requests []GroupJoinRequestResponse `json:"requests"`
}

// CommunitySubGroupResponse is one child group of a community.
//
// Narrower than GroupInfoResponse on purpose: the community listing answers
// with link targets, and emitting the other twenty keys at their zero value
// would tell a client the group has no owner and no topic, which was never
// measured.
type CommunitySubGroupResponse struct {
	JID               string `json:"jid"`
	Name              string `json:"name"`
	IsDefaultSubGroup bool   `json:"is_default_sub_group"`
}

// GetCommunitySubGroupsResponse is the body of `data` for the subgroups route.
type GetCommunitySubGroupsResponse struct {
	SubGroups []CommunitySubGroupResponse `json:"sub_groups"`
}

// GetCommunityParticipantsResponse is the body of `data` for the community
// participants route.
//
// A list of identities, and not of GroupParticipantResponse: this query answers
// with JIDs only, and the five extra keys would all be false or empty with no
// way for a client to tell that from a measured false.
type GetCommunityParticipantsResponse struct {
	Participants []string `json:"participants"`
}
