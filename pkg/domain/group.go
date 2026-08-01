// Package domain contém as entidades centrais do domínio disparazaap-wa-api.
package domain

// ListGroupsRequest representa a requisição para listar grupos
type ListGroupsRequest struct {
	// No body parameters
}

// ListGroupsResult representa o resultado da listagem de grupos
type ListGroupsResult struct {
	Groups interface{} `json:"groups"`
}

// GetGroupInfoRequest representa a requisição para obter informações de um grupo
type GetGroupInfoRequest struct {
	GroupJID string `json:"groupJID"`
}

// GetGroupInfoResult representa o resultado da obtenção de informações de grupo
type GetGroupInfoResult struct {
	GroupInfo interface{} `json:"group_info"`
}

// GetGroupInviteLinkRequest representa a requisição para obter link de convite do grupo
type GetGroupInviteLinkRequest struct {
	GroupJID string `json:"groupJID"`
}

// GetGroupInviteLinkResult representa o resultado da obtenção do link de convite
type GetGroupInviteLinkResult struct {
	InviteLink string `json:"invite_link"`
}

// GetGroupInviteInfoRequest representa a requisição para obter informações do convite
type GetGroupInviteInfoRequest struct {
	Code string `json:"Code"`
}

// GetGroupInviteInfoResult representa o resultado das informações do convite
type GetGroupInviteInfoResult struct {
	InviteInfo interface{} `json:"invite_info"`
}

// GroupJoinRequest representa a requisição para entrar em um grupo
type GroupJoinRequest struct {
	InviteLink string `json:"inviteLink"`
}

// GroupJoinResult representa o resultado de entrada no grupo
type GroupJoinResult struct {
	GroupJID string `json:"group_jid"`
	Details  string `json:"details"`
}

// GroupLeaveRequest representa a requisição para sair de um grupo
type GroupLeaveRequest struct {
	GroupJID string `json:"groupJID"`
}

// GroupLeaveResult representa o resultado de saída do grupo
type GroupLeaveResult struct {
	Details string `json:"details"`
}

// CreateGroupRequest representa a requisição para criar um grupo
type CreateGroupRequest struct {
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

// CreateGroupResult representa o resultado da criação de grupo
type CreateGroupResult struct {
	GroupInfo interface{} `json:"group_info"`
}

// UpdateGroupParticipantsRequest representa a requisição para atualizar participantes
type UpdateGroupParticipantsRequest struct {
	GroupJID string   `json:"groupJID"`
	Phone    []string `json:"Phone"`
	Action   string   `json:"Action"` // "add" or "remove"
}

// UpdateGroupParticipantsResult representa o resultado da atualização de participantes
type UpdateGroupParticipantsResult struct {
	Details string `json:"details"`
}

// SetGroupLockedRequest representa a requisição para bloquear/desbloquear grupo
type SetGroupLockedRequest struct {
	GroupJID string `json:"groupJID"`
	Locked   bool   `json:"locked"`
}

// SetGroupLockedResult representa o resultado do bloqueio/desbloqueio
type SetGroupLockedResult struct {
	Details string `json:"details"`
}

// SetGroupAnnounceRequest representa a requisição para definir modo de anúncio
type SetGroupAnnounceRequest struct {
	GroupJID string `json:"groupJID"`
	Announce bool   `json:"announce"`
}

// SetGroupAnnounceResult representa o resultado da definição do modo de anúncio
type SetGroupAnnounceResult struct {
	Details string `json:"details"`
}

// SetGroupNameRequest representa a requisição para renomear grupo
type SetGroupNameRequest struct {
	GroupJID string `json:"groupJID"`
	Name     string `json:"name"`
}

// SetGroupNameResult representa o resultado da renomeação
type SetGroupNameResult struct {
	Details string `json:"details"`
}

// SetGroupTopicRequest representa a requisição para definir descrição do grupo
type SetGroupTopicRequest struct {
	GroupJID string `json:"groupJID"`
	Topic    string `json:"topic"`
}

// SetGroupTopicResult representa o resultado da definição de descrição
type SetGroupTopicResult struct {
	Details string `json:"details"`
}

// SetGroupPhotoRequest representa a requisição para definir foto do grupo
type SetGroupPhotoRequest struct {
	GroupJID string `json:"groupJID"`
	Photo    string `json:"photo"`
}

// SetGroupPhotoResult representa o resultado da definição de foto
type SetGroupPhotoResult struct {
	Details string `json:"details"`
}

// RemoveGroupPhotoRequest representa a requisição para remover foto do grupo
type RemoveGroupPhotoRequest struct {
	GroupJID string `json:"groupJID"`
}

// RemoveGroupPhotoResult representa o resultado da remoção de foto
type RemoveGroupPhotoResult struct {
	Details string `json:"details"`
}

// SetDisappearingTimerRequest representa a requisição para definir timer de desaparecimento
type SetDisappearingTimerRequest struct {
	GroupJID string `json:"groupjid"`
	Duration string `json:"duration"` // "24h", "7d", "90d", "off"
}

// SetDisappearingTimerResult representa o resultado da definição do timer
type SetDisappearingTimerResult struct {
	Details string `json:"details"`
}
