// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// GetGroupRequestParticipantsRequest representa a requisição para listar participantes que solicitaram entrar
type GetGroupRequestParticipantsRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
}

func (r *GetGroupRequestParticipantsRequest) ResolveChat() {
	ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// GetGroupRequestParticipantsResult representa o resultado da listagem de participantes que solicitaram entrar
type GetGroupRequestParticipantsResult struct {
	// Response will be marshaled directly from wa-noise client response
	Details string      `json:"Details,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// UpdateGroupRequestParticipantsRequest representa a requisição para aprovar ou rejeitar participantes
type UpdateGroupRequestParticipantsRequest struct {
	ChatTarget
	GroupJID string   `json:"groupJID"`
	Phone    []string `json:"Phone"`
	Action   string   `json:"Action"` // approve, reject
}

func (r *UpdateGroupRequestParticipantsRequest) ResolveChat() {
	ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// UpdateGroupRequestParticipantsResult representa o resultado da atualização de participantes
type UpdateGroupRequestParticipantsResult struct {
	Details string `json:"Details"`
}

// SetGroupJoinApprovalModeRequest representa a requisição para definir modo de aprovação
type SetGroupJoinApprovalModeRequest struct {
	ChatTarget
	GroupJID string `json:"groupjid"`
	Mode     bool   `json:"mode"`
}

func (r *SetGroupJoinApprovalModeRequest) ResolveChat() {
	ResolveChatField(&r.GroupJID, r.ChatAlias)
}

// SetGroupJoinApprovalModeResult representa o resultado da definição do modo de aprovação
type SetGroupJoinApprovalModeResult struct {
	Details string `json:"Details"`
}
