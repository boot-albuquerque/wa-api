// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// The types in this file are use-case inputs and results, not the wire format.
// See pkg/presentation/http/dto/group and docs/HTTP-DTO-CONVENTIONS.md.

// GetGroupRequestParticipantsRequest is the input of the read-join-requests use case.
//
// No ChatTarget: the `chat` alias is resolved by the request DTO before this
// type is built.
type GetGroupRequestParticipantsRequest struct {
	GroupJID string
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
	GroupJID string
	Phone    []string
	Action   string // approve, reject
}

// UpdateGroupRequestParticipantsResult is the outcome of deciding join requests.
//
// F280: ParticipantsUpdate embutido, e não só Details — o mesmo formato que
// UpdateGroupParticipants (add/remove) já usa. Até 2026-08-28 esta struct só
// tinha Details, uma frase fixa, e o resultado por solicitante que o
// protocolo devolve (aprovado ou não, por quê) era descartado no adaptador —
// um sucesso parcial era indistinguível de sucesso total.
type UpdateGroupRequestParticipantsResult struct {
	Details string
	ParticipantsUpdate
}

// SetGroupJoinApprovalModeRequest is the input of the join-approval-mode use case.
type SetGroupJoinApprovalModeRequest struct {
	GroupJID string
	Mode     bool
}

// SetGroupJoinApprovalModeResult is the outcome of toggling join approval.
type SetGroupJoinApprovalModeResult struct {
	Details string
}
