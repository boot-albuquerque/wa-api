package group

import "wa-api/pkg/domain"

// The REQUEST bodies of the group and community family.
//
// They live here for the same reason the response types do: the naming rule of
// docs/HTTP-DTO-CONVENTIONS.md §8 covers request bodies too, and the domain
// structs these produce are use-case inputs that must be free to be renamed.
//
// Every field is a plain value, not a pointer: none of these routes needs to
// tell "absent" from "sent the zero" — a missing group_jid and an empty
// group_jid are refused by the same guard, and `announce: false` and an absent
// `announce` both mean "not announcement-only". A pointer here would buy a
// distinction nothing downstream reads (§6).
//
// `chat` is the alias every route with a destination accepts. It is resolved by
// domain.DecodeRequest, which calls ResolveChat after decoding, so a handler
// gets the destination already filled in.

// GroupTargetRequest is the body of every route whose only parameter is which
// group: leave, invite link, photo removal, and the join-request listing.
//
// One type instead of four identical ones. The four routes are not free to
// drift apart in this field — they all name the same thing — and four names
// would be four places to change if they ever did.
type GroupTargetRequest struct {
	GroupJID  string `json:"group_jid"`
	ChatAlias string `json:"chat"`
}

// ResolveChat fills the destination from the `chat` alias when it was not given
// directly.
func (r *GroupTargetRequest) ResolveChat() { domain.ResolveChatField(&r.GroupJID, r.ChatAlias) }

// ToInviteLinkDomain produces the invite-link use case input.
func (r GroupTargetRequest) ToInviteLinkDomain() domain.GetGroupInviteLinkRequest {
	return domain.GetGroupInviteLinkRequest{GroupJID: r.GroupJID}
}

// ToRequestParticipantsDomain produces the join-request listing input.
func (r GroupTargetRequest) ToRequestParticipantsDomain() domain.GetGroupRequestParticipantsRequest {
	return domain.GetGroupRequestParticipantsRequest{GroupJID: r.GroupJID}
}

// GetGroupInfoRequest is the body of the group-info route.
//
// H-DTO-GROUP-INFO: /group/info was the reference implementation of this
// migration and shipped with the response side done but the request side
// still decoding straight into domain.GetGroupInfoRequest, whose only `json`
// tag is `groupJID` — the one route in this family accepting a camelCase key.
// This DTO closes that gap the same way every sibling route already works.
type GetGroupInfoRequest struct {
	GroupJID  string `json:"group_jid"`
	ChatAlias string `json:"chat"`
}

func (r *GetGroupInfoRequest) ResolveChat() { domain.ResolveChatField(&r.GroupJID, r.ChatAlias) }

// ToDomain produces the use case input.
func (r GetGroupInfoRequest) ToDomain() domain.GetGroupInfoRequest {
	return domain.GetGroupInfoRequest{GroupJID: r.GroupJID}
}

// GetGroupInviteInfoRequest is the body of the invite-info route.
type GetGroupInviteInfoRequest struct {
	Code string `json:"code"`
}

// ToDomain produces the use case input.
func (r GetGroupInviteInfoRequest) ToDomain() domain.GetGroupInviteInfoRequest {
	return domain.GetGroupInviteInfoRequest{Code: r.Code}
}

// CreateGroupRequest is the body of the create-group route.
type CreateGroupRequest struct {
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
	// IsParent creates a community. LinkedParentJID creates the group INSIDE
	// one. They are mutually exclusive, and the route says so.
	IsParent        bool   `json:"is_parent"`
	LinkedParentJID string `json:"linked_parent_jid"`
}

// ToDomainOpts produces the creation flags.
func (r CreateGroupRequest) ToDomainOpts() domain.CreateGroupOpts {
	return domain.CreateGroupOpts{
		IsParent:        r.IsParent,
		LinkedParentJID: domain.JID(r.LinkedParentJID),
	}
}

// JoinGroupRequest is the body of the join-group route.
type JoinGroupRequest struct {
	Code string `json:"code"`
}

// SetGroupNameRequest is the body of the rename route.
type SetGroupNameRequest struct {
	GroupJID  string `json:"group_jid"`
	Name      string `json:"name"`
	ChatAlias string `json:"chat"`
}

func (r *SetGroupNameRequest) ResolveChat() { domain.ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupTopicRequest is the body of the description route.
type SetGroupTopicRequest struct {
	GroupJID  string `json:"group_jid"`
	Topic     string `json:"topic"`
	ChatAlias string `json:"chat"`
}

func (r *SetGroupTopicRequest) ResolveChat() { domain.ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupPhotoRequest is the body of the photo route. Photo is base64.
type SetGroupPhotoRequest struct {
	GroupJID  string `json:"group_jid"`
	Photo     string `json:"photo"`
	ChatAlias string `json:"chat"`
}

func (r *SetGroupPhotoRequest) ResolveChat() { domain.ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupAnnounceRequest is the body of the announcement-mode route.
type SetGroupAnnounceRequest struct {
	GroupJID  string `json:"group_jid"`
	Announce  bool   `json:"announce"`
	ChatAlias string `json:"chat"`
}

func (r *SetGroupAnnounceRequest) ResolveChat() { domain.ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupLockedRequest is the body of the locked-settings route.
type SetGroupLockedRequest struct {
	GroupJID  string `json:"group_jid"`
	Locked    bool   `json:"locked"`
	ChatAlias string `json:"chat"`
}

func (r *SetGroupLockedRequest) ResolveChat() { domain.ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetDisappearingTimerRequest is the body of the ephemeral-messages route.
// Duration is one of "24h", "7d", "90d"; anything else turns the timer off.
type SetDisappearingTimerRequest struct {
	GroupJID  string `json:"group_jid"`
	Duration  string `json:"duration"`
	ChatAlias string `json:"chat"`
}

func (r *SetDisappearingTimerRequest) ResolveChat() {
	domain.ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// UpdateGroupParticipantsRequest is the body of the add/remove/promote/demote
// members route.
type UpdateGroupParticipantsRequest struct {
	GroupJID  string   `json:"group_jid"`
	Phone     []string `json:"phone"`
	Action    string   `json:"action"` // "add", "remove", "promote" or "demote" (F263)
	ChatAlias string   `json:"chat"`
}

func (r *UpdateGroupParticipantsRequest) ResolveChat() {
	domain.ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// UpdateGroupRequestParticipantsRequest is the body of the approve/reject
// join-requests route.
type UpdateGroupRequestParticipantsRequest struct {
	GroupJID  string   `json:"group_jid"`
	Phone     []string `json:"phone"`
	Action    string   `json:"action"` // "approve" or "reject"
	ChatAlias string   `json:"chat"`
}

func (r *UpdateGroupRequestParticipantsRequest) ResolveChat() {
	domain.ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// ToDomain produces the use case input.
func (r UpdateGroupRequestParticipantsRequest) ToDomain() domain.UpdateGroupRequestParticipantsRequest {
	return domain.UpdateGroupRequestParticipantsRequest{
		GroupJID: r.GroupJID,
		Phone:    r.Phone,
		Action:   r.Action,
	}
}

// SetGroupJoinApprovalModeRequest is the body of the join-approval route.
type SetGroupJoinApprovalModeRequest struct {
	GroupJID  string `json:"group_jid"`
	Mode      bool   `json:"mode"`
	ChatAlias string `json:"chat"`
}

func (r *SetGroupJoinApprovalModeRequest) ResolveChat() {
	domain.ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// ToDomain produces the use case input.
func (r SetGroupJoinApprovalModeRequest) ToDomain() domain.SetGroupJoinApprovalModeRequest {
	return domain.SetGroupJoinApprovalModeRequest{GroupJID: r.GroupJID, Mode: r.Mode}
}

// CommunityTargetRequest is the body of the community read routes.
type CommunityTargetRequest struct {
	CommunityJID string `json:"community_jid"`
	ChatAlias    string `json:"chat"`
}

func (r *CommunityTargetRequest) ResolveChat() {
	domain.ResolveChatField(&r.CommunityJID, r.ChatAlias)
}

// ToSubGroupsDomain produces the subgroups use case input.
func (r CommunityTargetRequest) ToSubGroupsDomain() domain.GetCommunitySubGroupsRequest {
	return domain.GetCommunitySubGroupsRequest{CommunityJID: r.CommunityJID}
}

// ToParticipantsDomain produces the community participants use case input.
func (r CommunityTargetRequest) ToParticipantsDomain() domain.GetCommunityParticipantsRequest {
	return domain.GetCommunityParticipantsRequest{CommunityJID: r.CommunityJID}
}

// CommunityLinkRequest is the body of both the link and the unlink route: the
// two name the same pair, and the verb is the path.
type CommunityLinkRequest struct {
	CommunityJID string `json:"community_jid"`
	GroupJID     string `json:"group_jid"`
}

// ToLinkDomain produces the link use case input.
func (r CommunityLinkRequest) ToLinkDomain() domain.CommunityLinkRequest {
	return domain.CommunityLinkRequest{CommunityJID: r.CommunityJID, GroupJID: r.GroupJID}
}

// ToUnlinkDomain produces the unlink use case input.
func (r CommunityLinkRequest) ToUnlinkDomain() domain.CommunityUnlinkRequest {
	return domain.CommunityUnlinkRequest{CommunityJID: r.CommunityJID, GroupJID: r.GroupJID}
}
