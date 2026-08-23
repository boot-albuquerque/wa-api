package port

import (
	"context"
	"time"

	"wa-api/pkg/domain"
)

// GroupDirectory expõe as operações de leitura sobre grupos.
//
// Os resultados são any porque os tipos de resposta do domínio
// (domain.GetGroupInfoResult.GroupInfo, domain.ListGroupsResult.Groups) já
// eram interface{} antes desta fase — o valor atravessa o use case opaco e é
// serializado no handler. Tipá-los em domínio é trabalho de arq/F5, que a
// ADR-001 declara fora deste plano.
type GroupDirectory interface {
	SessionGuard

	// GetGroupInfo devolve os metadados de um grupo.
	GetGroupInfo(ctx context.Context, txtID string, group domain.JID) (any, error)

	// GetGroupInfoFromLink devolve os metadados a partir de um código de
	// convite.
	GetGroupInfoFromLink(ctx context.Context, txtID, code string) (any, error)

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
	ListJoinedGroups(ctx context.Context, txtID string) (any, int, error)
}

// GroupLifecycle cobre entrar e sair de grupos, e criá-los.
type GroupLifecycle interface {
	SessionGuard

	// CreateGroup cria um grupo com os participantes informados.
	CreateGroup(ctx context.Context, txtID, name string, participants []domain.JID) (any, error)

	// JoinGroup entra num grupo por código de convite.
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

// GroupRequests cobre a fila de solicitações de entrada em grupo.
type GroupRequests interface {
	SessionGuard

	// GetRequestParticipants lista quem solicitou entrar no grupo.
	GetRequestParticipants(ctx context.Context, txtID string, group domain.JID) (any, error)

	// UpdateRequestParticipants aprova ou rejeita solicitações.
	UpdateRequestParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) error

	// SetJoinApprovalMode liga/desliga a exigência de aprovação.
	SetJoinApprovalMode(ctx context.Context, txtID string, group domain.JID, mode bool) error
}
