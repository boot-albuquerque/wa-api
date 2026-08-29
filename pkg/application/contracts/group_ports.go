package port

import (
	"context"
	"time"

	"wa-api/pkg/domain"
)

// GroupDirectory expõe as operações de leitura sobre grupos.
//
// Todas as leituras são TIPADAS. Enquanto eram `any`, era o MOTOR da sessão que
// decidia a forma do JSON servido ao cliente, e nenhuma das formas estava
// declarada em lado nenhum. Cada adaptador normaliza para o tipo de domínio, e
// a fronteira HTTP apresenta a partir daí.
type GroupDirectory interface {
	SessionGuard

	// GetGroupInfo devolve os metadados de um grupo.
	//
	// TIPADO, e não `any`: com `any` era o motor da sessão que decidia a
	// forma do JSON servido ao cliente — o struct de protocolo do wa-noise
	// numa sessão, o struct de conversa do headless na outra — e nenhuma das
	// duas estava declarada em lado nenhum. Cada adaptador normaliza para
	// domain.GroupInfo, e a fronteira HTTP apresenta a partir daí.
	GetGroupInfo(ctx context.Context, txtID string, group domain.JID) (*domain.GroupInfo, error)

	// GetGroupInfoFromLink devolve os metadados a partir de um código de
	// convite, SEM entrar no grupo.
	//
	// Mesmo tipo de GetGroupInfo, e não um tipo próprio mais estreito: os dois
	// motores respondem com metadados de grupo, e o que um motor não observa
	// fica no zero — a regra que domain.GroupInfo já documenta.
	GetGroupInfoFromLink(ctx context.Context, txtID, code string) (*domain.GroupInfo, error)

	// GetGroupInviteLink devolve o link de convite de um grupo.
	GetGroupInviteLink(ctx context.Context, txtID string, group domain.JID) (string, error)

	// GroupNames devolve o nome de cada grupo de que a sessão participa,
	// por JID, numa ÚNICA chamada.
	//
	// A alternativa — GetGroupInfo por grupo — custa um round-trip por
	// grupo, e a lista de conversas precisa de todos de uma vez. Tipado pelo
	// mesmo motivo de ContactNames.
	GroupNames(ctx context.Context, txtID string) (map[domain.JID]string, error)

	// ListJoinedGroups devolve os grupos de que a sessão participa, e a
	// contagem, que o use case usa para logar.
	ListJoinedGroups(ctx context.Context, txtID string) ([]*domain.GroupInfo, int, error)
}

// GroupLifecycle cobre entrar e sair de grupos, e criá-los.
type GroupLifecycle interface {
	SessionGuard

	// CreateGroup creates a group (or community) with the given participants.
	// opts carries optional community flags (IsParent, LinkedParentJID).
	//
	// Devolve domain.CreatedGroup e não só o grupo: um dos motores não CRIA —
	// a capability headless chama-se Ensure e devolve o grupo homónimo que já
	// existia em vez de o duplicar. Achatar isso faria duas chamadas iguais
	// parecerem ter criado dois grupos.
	CreateGroup(ctx context.Context, txtID, name string, participants []domain.JID, opts domain.CreateGroupOpts) (*domain.CreatedGroup, error)

	// JoinGroup entra num grupo por código de convite.
	//
	// O resultado continua `any` porque NÃO atravessa fronteira pública
	// nenhuma: a rota /group/join descarta-o e responde só com a confirmação.
	// Um DTO para um valor que ninguém serializa seria uma camada a manter sem
	// nada a proteger (docs/HTTP-DTO-CONVENTIONS.md §3).
	JoinGroup(ctx context.Context, txtID, code string) (any, error)

	// LeaveGroup sai de um grupo.
	LeaveGroup(ctx context.Context, txtID string, group domain.JID) error
}

// As escritas sobre um grupo do qual a sessão já participa, em QUATRO portas.
//
// # Por que quatro, e de onde veio a costura (decisões 87 e 92)
//
// A costura NÃO veio do uso, e isso está escrito porque contraria o precedente
// da decisão 82. Ali, o ContactDirectory foi dividido depois de medir nove
// casos de uso, dos quais SETE precisavam de um método só — os dados desenharam
// a divisão. Aqui há um único consumidor, e cada método público dele usa
// exatamente UM método da porta: um para um, sem exceção. O uso não desenha
// costura nenhuma.
//
// A costura é de CAPACIDADE DE TRANSPORTE, e é legítima porque portas existem
// para ser satisfeitas por transportes. Com a divisão, o ponto de montagem
// deixa de compilar até alguém dizer qual metade quer — em vez de o chamador
// descobrir em tempo de execução que o transporte não faz aquilo.
//
// O código já pedia a divisão antes de alguém a decidir: DOIS adaptadores
// headless implementavam pedaços desta porta, e NENHUM podia declarar que a
// satisfazia, porque nenhum a satisfazia inteira.

// GroupInfoSettings cobre as configurações que o grupo guarda sobre si mesmo.
type GroupInfoSettings interface {
	SessionGuard

	// SetGroupName renomeia o grupo.
	SetGroupName(ctx context.Context, txtID string, group domain.JID, name string) error

	// SetGroupTopic define a descrição do grupo.
	SetGroupTopic(ctx context.Context, txtID string, group domain.JID, topic string) error

	// SetGroupAnnounce liga/desliga o modo somente-administradores.
	SetGroupAnnounce(ctx context.Context, txtID string, group domain.JID, announce bool) error

	// SetGroupLocked tranca/destranca as configurações do grupo.
	SetGroupLocked(ctx context.Context, txtID string, group domain.JID, locked bool) error
}

// GroupParticipants cobre quem está no grupo.
type GroupParticipants interface {
	SessionGuard

	// UpdateGroupParticipants adiciona ou remove participantes.
	//
	// Devolve um desfecho TIPADO, e não um `any`, porque há transporte em que a
	// mudança chega ao servidor e a sessão que agiu NÃO consegue lê-la de volta
	// (medido: H58 e H65 do stack headless). Sem um lugar para dizer isso, o
	// adaptador teria de escolher entre mentir sucesso e mentir falha.
	UpdateGroupParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.ParticipantAction) (domain.ParticipantsUpdate, error)
}

// GroupPhotoSetter cobre a foto do grupo.
//
// Porta própria porque há transporte que NÃO a serve, e a razão é medida: no
// build headless os módulos de foto da página estão ausentes (H140). O nome diz
// o que ela faz, não por que um transporte não a faz — uma porta chamada pelo
// motivo da recusa envelheceria no dia em que a recusa deixasse de valer.
type GroupPhotoSetter interface {
	SessionGuard

	// SetGroupPhoto define a foto do grupo. photo nil remove a foto — é
	// como o upstream já implementava RemoveGroupPhoto, e por isso não há
	// um método separado para remover.
	SetGroupPhoto(ctx context.Context, txtID string, group domain.JID, photo []byte) error
}

// GroupEphemeralSetter cobre a expiração de mensagens.
//
// Porta própria pela mesma razão de forma que GroupPhotoSetter, e por uma razão
// de conteúdo DIFERENTE: aqui não há medição nenhuma no stack headless. Não é
// capacidade fechada, é capacidade por medir, e juntar as duas na mesma porta
// faria as duas parecerem a mesma coisa.
type GroupEphemeralSetter interface {
	SessionGuard

	// SetDisappearingTimer define o tempo de expiração das mensagens.
	SetDisappearingTimer(ctx context.Context, txtID string, group domain.JID, d time.Duration, at time.Time) error
}

// GroupSettings é a COMPOSIÇÃO das quatro, para um transporte que serve todas
// declarar isso numa linha só. Foi o que a decisão 82 fez com ContactDirectory.
type GroupSettings interface {
	GroupInfoSettings
	GroupParticipants
	GroupPhotoSetter
	GroupEphemeralSetter
}

// CommunityDirectory exposes read operations on communities (parent groups).
type CommunityDirectory interface {
	SessionGuard

	// GetSubGroups returns the child groups of a community.
	GetSubGroups(ctx context.Context, txtID string, community domain.JID) ([]domain.CommunitySubGroup, error)

	// GetLinkedGroupsParticipants returns participants across all child groups.
	//
	// Identidades, e nada mais: é com isso que o servidor responde a esta
	// consulta.
	GetLinkedGroupsParticipants(ctx context.Context, txtID string, community domain.JID) ([]domain.JID, error)
}

// CommunityLifecycle covers linking and unlinking groups to/from communities.
type CommunityLifecycle interface {
	SessionGuard

	// LinkGroup adds an existing group as a child of a community.
	LinkGroup(ctx context.Context, txtID string, parent, child domain.JID) error

	// UnlinkGroup removes a child group from a community.
	UnlinkGroup(ctx context.Context, txtID string, parent, child domain.JID) error
}

// GroupRequests cobre a fila de solicitações de entrada em grupo.
type GroupRequests interface {
	SessionGuard

	// GetRequestParticipants lista quem solicitou entrar no grupo.
	GetRequestParticipants(ctx context.Context, txtID string, group domain.JID) ([]domain.GroupJoinRequest, error)

	// UpdateRequestParticipants aprova ou rejeita solicitações.
	//
	// F280: o resultado por solicitante que o protocolo devolve (aprovado
	// ou não, e o motivo quando não) já não é descartado — sai em
	// domain.ParticipantsUpdate, no mesmo formato que UpdateGroupParticipants
	// já usa para adicionar/remover membros. Um sucesso parcial passa a ser
	// distinguível de sucesso total.
	UpdateRequestParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) (domain.ParticipantsUpdate, error)

	// SetJoinApprovalMode liga/desliga a exigência de aprovação.
	SetJoinApprovalMode(ctx context.Context, txtID string, group domain.JID, mode bool) error
}
