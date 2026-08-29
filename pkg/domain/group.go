package domain

import (
	"errors"
	"time"
)

// The types in this file are use-case inputs and results. They are NOT the
// wire format: the public shape of every group route lives in
// pkg/presentation/http/dto/group, and the mapping between the two is
// hand-written. See docs/HTTP-DTO-CONVENTIONS.md.
//
// Hence: Go-idiomatic names, and no `json` tags.

// ListGroupsRequest is the input of the list-groups use case. It carries no
// parameter today, and exists so the use case signature does not have to change
// when it does.
type ListGroupsRequest struct{}

// ListGroupsResult is every group this session belongs to.
//
// TYPED, and no longer `any`: with `any` it was the session's ENGINE that
// decided the JSON a client received — the wa-noise protocol struct on one
// session, the headless conversation struct on another — and neither shape was
// declared anywhere. Same reasoning as GetGroupInfoResult (see group_info.go).
type ListGroupsResult struct {
	Groups []*GroupInfo
}

// GetGroupInfoRequest representa a requisição para obter informações de um grupo
//
// F267: GroupJID carried a dead `json:"groupJID"` tag left over from before
// the DTO migration. Nothing decodes JSON into this type — the wire shape is
// pkg/presentation/http/dto/group.GetGroupInfoRequest
// (handler_group.go:170) — so the tag was never live; it was exactly the trap
// this file's own package comment warns about, misleading anything that
// walked pkg/domain to infer the route's contract. See
// group_no_wire_tags_test.go.
type GetGroupInfoRequest struct {
	ChatTarget
	GroupJID string
}

func (r *GetGroupInfoRequest) ResolveChat() { ResolveChatField(&r.GroupJID, r.ChatAlias) }

// GetGroupInfoResult representa o resultado da obtenção de informações de grupo.
//
// Sem tag `json`: este tipo já NÃO é o formato de fio. Quem o serializa é
// pkg/presentation/http/dto/group.PresentGetGroupInfo.
type GetGroupInfoResult struct {
	GroupInfo *GroupInfo
}

// GetGroupInviteLinkRequest is the input of the invite-link use case.
//
// No ChatTarget: the `chat` alias is a WIRE concern, and it is resolved by the
// request DTO before this type is built (dto/group.GroupTargetRequest).
type GetGroupInviteLinkRequest struct {
	GroupJID string
}

// GetGroupInviteLinkResult is the invite link of a group.
type GetGroupInviteLinkResult struct {
	InviteLink string
}

// GetGroupInviteInfoRequest is the input of the invite-info use case.
type GetGroupInviteInfoRequest struct {
	Code string
}

// GetGroupInviteInfoResult is what a link says about a group WITHOUT joining it.
//
// It reuses GroupInfo instead of declaring a second, narrower type: both
// engines answer with group metadata (wa-noise with the full protocol struct,
// headless with jid/subject/size/approval), and what an engine cannot observe
// stays at its zero value — exactly the rule GroupInfo already documents.
type GetGroupInviteInfoResult struct {
	InviteInfo *GroupInfo
}

// CreateGroupOpts carries optional flags for group creation that alter the
// kind of group being created. Zero value = normal group.
type CreateGroupOpts struct {
	// IsParent creates a community instead of a normal group. When true the
	// WhatsApp server creates the announcement sub-group automatically.
	IsParent bool
	// LinkedParentJID, when set, creates the group as a child of the given
	// community. Mutually exclusive with IsParent.
	LinkedParentJID JID
}

// CreatedGroup is the outcome of asking for a group to exist.
//
// Created is carried, and not assumed true, because one of the two engines does
// not CREATE: the headless capability is `Ensure`, and a group with the same
// name that already exists is returned instead of duplicated. Flattening that
// would make two identical calls look like they made two groups when they made
// one. The wa-noise transport always creates, and reports true.
type CreatedGroup struct {
	Group   *GroupInfo
	Created bool
}

// ParticipantAction é a mudança pedida sobre a lista de participantes de um
// grupo. Os valores são os que o payload já aceitava.
type ParticipantAction string

const (
	// ParticipantAdd adiciona participantes ao grupo.
	ParticipantAdd ParticipantAction = "add"
	// ParticipantRemove remove participantes do grupo.
	ParticipantRemove ParticipantAction = "remove"
	// ParticipantPromote torna participantes administradores do grupo (F263).
	ParticipantPromote ParticipantAction = "promote"
	// ParticipantDemote retira o cargo de administrador de participantes (F263).
	ParticipantDemote ParticipantAction = "demote"
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
	// Participants is the roster the transport read back, when it can read one.
	// TYPED, and no longer `any`: the wa-noise transport answers with the
	// resulting participant list on the same call, and that list used to reach
	// the wire as the protocol struct's Go field names. The headless transport
	// observes nothing here and leaves it empty — which is what Confirmed and
	// Reason are for.
	Participants []GroupParticipant
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

// GroupJoinRequest is one pending request to join a group.
//
// The union of what the two engines see: wa-noise reports the requester and
// when the request was made; the headless page also reports who tried to add
// them and by which method. What an engine does not observe stays zero.
type GroupJoinRequest struct {
	// JID is who asked to join.
	JID JID
	// RequestedAt is when the request was made. Zero when the engine does not
	// report it.
	RequestedAt time.Time
	// AddedByJID is who put them there, when there is such a person: a request
	// made by following an invite link has no adder.
	AddedByJID JID
	// Method is HOW the request arrived — measured as "InviteLink" for a
	// request made by following a link. It distinguishes a stranger with a link
	// from somebody a member tried to add, which is the difference an admin
	// actually decides on.
	Method string
}
