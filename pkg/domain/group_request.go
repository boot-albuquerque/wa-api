// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// GetGroupRequestParticipantsRequest representa a requisição para listar participantes que solicitaram entrar
type GetGroupRequestParticipantsRequest struct {
	GroupJID string `json:"groupJID"`
}

// GetGroupRequestParticipantsResult representa o resultado da listagem de participantes que solicitaram entrar
type GetGroupRequestParticipantsResult struct {
	// Response will be marshaled directly from whatsmeow client response
	Details string      `json:"Details,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// UpdateGroupRequestParticipantsRequest representa a requisição para aprovar ou rejeitar participantes
type UpdateGroupRequestParticipantsRequest struct {
	GroupJID string   `json:"groupJID"`
	Phone    []string `json:"Phone"`
	Action   string   `json:"Action"` // approve, reject
}

// UpdateGroupRequestParticipantsResult representa o resultado da atualização de participantes
type UpdateGroupRequestParticipantsResult struct {
	Details string `json:"Details"`
}

// SetGroupJoinApprovalModeRequest representa a requisição para definir modo de aprovação
type SetGroupJoinApprovalModeRequest struct {
	GroupJID string `json:"groupjid"`
	Mode     bool   `json:"mode"`
}

// SetGroupJoinApprovalModeResult representa o resultado da definição do modo de aprovação
type SetGroupJoinApprovalModeResult struct {
	Details string `json:"Details"`
}
