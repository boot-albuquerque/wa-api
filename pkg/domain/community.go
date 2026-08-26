package domain

// GetCommunitySubGroupsRequest is the request to list subgroups of a community.
type GetCommunitySubGroupsRequest struct {
	ChatTarget
	CommunityJID string `json:"communityJID"`
}

func (r *GetCommunitySubGroupsRequest) ResolveChat() { ResolveChatField(&r.CommunityJID, r.ChatAlias) }

// GetCommunitySubGroupsResult wraps the subgroups response.
type GetCommunitySubGroupsResult struct {
	SubGroups any `json:"sub_groups"`
}

// GetCommunityParticipantsRequest is the request to list participants of linked groups.
type GetCommunityParticipantsRequest struct {
	ChatTarget
	CommunityJID string `json:"communityJID"`
}

func (r *GetCommunityParticipantsRequest) ResolveChat() {
	ResolveChatField(&r.CommunityJID, r.ChatAlias)
}

// GetCommunityParticipantsResult wraps the linked groups participants response.
type GetCommunityParticipantsResult struct {
	Participants any `json:"participants"`
}

// CommunityLinkRequest is the request to link a group to a community.
type CommunityLinkRequest struct {
	CommunityJID string `json:"communityJID"`
	GroupJID     string `json:"groupJID"`
}

// CommunityUnlinkRequest is the request to unlink a group from a community.
type CommunityUnlinkRequest struct {
	CommunityJID string `json:"communityJID"`
	GroupJID     string `json:"groupJID"`
}
