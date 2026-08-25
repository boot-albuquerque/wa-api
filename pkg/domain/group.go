package domain

import "errors"

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
	ChatTarget
	GroupJID string `json:"groupJID"`
}

func (r *GetGroupInfoRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// GetGroupInfoResult representa o resultado da obtenção de informações de grupo
type GetGroupInfoResult struct {
	GroupInfo interface{} `json:"group_info"`
}

// GetGroupInviteLinkRequest representa a requisição para obter link de convite do grupo
type GetGroupInviteLinkRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
}

func (r *GetGroupInviteLinkRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

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
	ChatTarget
	GroupJID string `json:"groupJID"`
}

func (r *GroupLeaveRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

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
	ChatTarget
	GroupJID string   `json:"groupJID"`
	Phone    []string `json:"Phone"`
	Action   string   `json:"Action"` // "add" or "remove"
}

func (r *UpdateGroupParticipantsRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// UpdateGroupParticipantsResult representa o resultado da atualização de participantes
type UpdateGroupParticipantsResult struct {
	Details string `json:"details"`
}

// SetGroupLockedRequest representa a requisição para bloquear/desbloquear grupo
type SetGroupLockedRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
	Locked   bool   `json:"locked"`
}

func (r *SetGroupLockedRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupLockedResult representa o resultado do bloqueio/desbloqueio
type SetGroupLockedResult struct {
	Details string `json:"details"`
}

// SetGroupAnnounceRequest representa a requisição para definir modo de anúncio
type SetGroupAnnounceRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
	Announce bool   `json:"announce"`
}

func (r *SetGroupAnnounceRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupAnnounceResult representa o resultado da definição do modo de anúncio
type SetGroupAnnounceResult struct {
	Details string `json:"details"`
}

// SetGroupNameRequest representa a requisição para renomear grupo
type SetGroupNameRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
	Name     string `json:"name"`
}

func (r *SetGroupNameRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupNameResult representa o resultado da renomeação
type SetGroupNameResult struct {
	Details string `json:"details"`
}

// SetGroupTopicRequest representa a requisição para definir descrição do grupo
type SetGroupTopicRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
	Topic    string `json:"topic"`
}

func (r *SetGroupTopicRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupTopicResult representa o resultado da definição de descrição
type SetGroupTopicResult struct {
	Details string `json:"details"`
}

// SetGroupPhotoRequest representa a requisição para definir foto do grupo
type SetGroupPhotoRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
	Photo    string `json:"photo"`
}

func (r *SetGroupPhotoRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetGroupPhotoResult representa o resultado da definição de foto
type SetGroupPhotoResult struct {
	Details string `json:"details"`
}

// RemoveGroupPhotoRequest representa a requisição para remover foto do grupo
type RemoveGroupPhotoRequest struct {
	ChatTarget
	GroupJID string `json:"groupJID"`
}

func (r *RemoveGroupPhotoRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// RemoveGroupPhotoResult representa o resultado da remoção de foto
type RemoveGroupPhotoResult struct {
	Details string `json:"details"`
}

// SetDisappearingTimerRequest representa a requisição para definir timer de desaparecimento
type SetDisappearingTimerRequest struct {
	ChatTarget
	GroupJID string `json:"groupjid"`
	Duration string `json:"duration"` // "24h", "7d", "90d", "off"
}

func (r *SetDisappearingTimerRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// SetDisappearingTimerResult representa o resultado da definição do timer
type SetDisappearingTimerResult struct {
	Details string `json:"details"`
}

// ParticipantAction é a mudança pedida sobre a lista de participantes de um
// grupo. Os valores são os que o payload já aceitava.
type ParticipantAction string

const (
	// ParticipantAdd adiciona participantes ao grupo.
	ParticipantAdd ParticipantAction = "add"
	// ParticipantRemove remove participantes do grupo.
	ParticipantRemove ParticipantAction = "remove"
)

// RequestAction é o veredito sobre uma solicitação de entrada em grupo.
type RequestAction string

const (
	// RequestApprove aprova a solicitação.
	RequestApprove RequestAction = "approve"
	// RequestReject rejeita a solicitação.
	RequestReject RequestAction = "reject"
)

// ParticipantsUpdate é o desfecho de mudar participantes de um grupo.
//
// # Por que carrega uma confirmação em vez de só o resultado
//
// A invariante 14 do stack headless exige que nenhuma escrita devolva sucesso
// silencioso: quem escreve lê a pós-condição de volta. As operações de
// participante são o único lugar onde isso É IMPOSSÍVEL para quem age, e não por
// falta de esforço — foi MEDIDO (H58 para adicionar e remover, H65 para promover
// e rebaixar): a mudança CHEGA ao servidor, e a sessão que agiu não a vê. A
// confirmação só aparece em OUTRA sessão.
//
// Havia três saídas e duas eram ruins. Devolver sucesso sem pós-condição abriria
// exceção à invariante, que existe justamente contra isso. Ler de volta assim
// mesmo reportaria FALHA para uma mudança bem-sucedida, que é pior que não ler.
//
// A terceira, que é esta: o "não confirmável" deixa de ser silêncio e passa a ser
// um DESFECHO RELATADO. A invariante sobrevive porque ela proíbe o sucesso mudo,
// não a incerteza declarada.
//
// E o campo serve aos dois transportes: dá ao socket um lugar para dizer quando
// ELE não conseguiu confirmar — o que hoje não tem onde ser dito.
type ParticipantsUpdate struct {
	// Result é o que o transporte devolveu, como antes.
	Result any
	// Confirmed diz que a sessão que agiu LEU a mudança de volta.
	Confirmed bool
	// Reason explica por que não, e é obrigatória quando Confirmed é falso.
	// Vazia com Confirmed falso seria o sucesso silencioso de volta, agora
	// disfarçado de estrutura.
	Reason string
}

// Valida recusa o desfecho que traria o sucesso silencioso de volta.
//
// Vive como método, e não como comentário, para que quem escreva um adaptador
// novo o ENCONTRE — um comentário depende de alguém o ler antes de errar.
func (u ParticipantsUpdate) Valida() error {
	if !u.Confirmed && u.Reason == "" {
		return errors.New("domain: participantes atualizados sem confirmação e sem motivo escrito")
	}
	return nil
}
