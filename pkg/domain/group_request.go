// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// The types in this file are use-case inputs and results, not the wire format.
// See pkg/presentation/http/dto/group and docs/HTTP-DTO-CONVENTIONS.md.

// GetGroupRequestParticipantsRequest is the input of the read-join-requests use case.
type GetGroupRequestParticipantsRequest struct {
	ChatTarget
	GroupJID string
}

func (r *GetGroupRequestParticipantsRequest) ResolveChat() {
	ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// GetGroupRequestParticipantsResult is the pending join requests of one group.
//
// TYPED, and no longer a json.RawMessage produced by re-marshalling whatever
// the engine returned: that made the ENGINE the author of the public payload,
// with the protocol struct's Go field names as keys.
type GetGroupRequestParticipantsResult struct {
	Requests []GroupJoinRequest
}

// UpdateGroupRequestParticipantsRequest is the input of the decide-join-requests use case.
type UpdateGroupRequestParticipantsRequest struct {
	ChatTarget
	GroupJID string
	Phone    []string
	Action   string // approve, reject
}

func (r *UpdateGroupRequestParticipantsRequest) ResolveChat() {
	ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// UpdateGroupRequestParticipantsResult is the outcome of deciding join requests.
type UpdateGroupRequestParticipantsResult struct {
	Details string
}

// SetGroupJoinApprovalModeRequest is the input of the join-approval-mode use case.
type SetGroupJoinApprovalModeRequest struct {
	ChatTarget
	GroupJID string
	Mode     bool
}

func (r *SetGroupJoinApprovalModeRequest) ResolveChat() {
	ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// SetGroupJoinApprovalModeResult is the outcome of toggling join approval.
type SetGroupJoinApprovalModeResult struct {
	Details string
}
