package domain

// The types in this file are use-case inputs and results, not the wire format.
// See pkg/presentation/http/dto/group and docs/HTTP-DTO-CONVENTIONS.md.

// GetCommunitySubGroupsRequest is the request to list subgroups of a community.
type GetCommunitySubGroupsRequest struct {
	ChatTarget
	CommunityJID string
}

func (r *GetCommunitySubGroupsRequest) ResolveChat() { ResolveChatField(&r.CommunityJID, r.ChatAlias) }

// CommunitySubGroup is one child group of a community, as the community
// listing reports it — which is LESS than GroupInfo: the server answers this
// query with the link targets only (jid, name, and whether it is the default
// announcement group), not the full metadata of each child.
type CommunitySubGroup struct {
	JID  JID
	Name string
	// IsDefaultSubGroup marks the announcement group the server creates with
	// every community.
	IsDefaultSubGroup bool
}

// GetCommunitySubGroupsResult wraps the subgroups response.
type GetCommunitySubGroupsResult struct {
	SubGroups []CommunitySubGroup
}

// GetCommunityParticipantsRequest is the request to list participants of linked groups.
type GetCommunityParticipantsRequest struct {
	ChatTarget
	CommunityJID string
}

func (r *GetCommunityParticipantsRequest) ResolveChat() {
	ResolveChatField(&r.CommunityJID, r.ChatAlias)
}

// GetCommunityParticipantsResult is who is in the community's linked groups.
//
// Identities only: this query answers with JIDs, and nothing else about each
// participant — no admin flag, no display name. Presenting it as a
// GroupParticipant list would emit five keys the server never told us about,
// all of them false or empty, which a client cannot tell from "measured false".
type GetCommunityParticipantsResult struct {
	Participants []JID
}

// CommunityLinkRequest is the request to link a group to a community.
type CommunityLinkRequest struct {
	CommunityJID string
	GroupJID     string
}

// CommunityUnlinkRequest is the request to unlink a group from a community.
type CommunityUnlinkRequest struct {
	CommunityJID string
	GroupJID     string
}
