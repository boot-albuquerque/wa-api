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

// GroupSettings cobre as escritas sobre um grupo do qual a sessão já
// participa.
//
// Separada de GroupLifecycle não por estética: juntas passariam de dez
// métodos, o limite a partir do qual o próprio plano manda parar e
// reconsiderar a fronteira da interface.
type GroupSettings interface {
	SessionGuard

	// SetGroupName renomeia o grupo.
	SetGroupName(ctx context.Context, txtID string, group domain.JID, name string) error

	// SetGroupTopic define a descrição do grupo.
	SetGroupTopic(ctx context.Context, txtID string, group domain.JID, topic string) error

	// SetGroupPhoto define a foto do grupo. photo nil remove a foto — é
	// como o upstream já implementava RemoveGroupPhoto, e por isso não há
	// um método separado para remover.
	SetGroupPhoto(ctx context.Context, txtID string, group domain.JID, photo []byte) error

	// SetGroupAnnounce liga/desliga o modo somente-administradores.
	SetGroupAnnounce(ctx context.Context, txtID string, group domain.JID, announce bool) error

	// SetGroupLocked tranca/destranca as configurações do grupo.
	SetGroupLocked(ctx context.Context, txtID string, group domain.JID, locked bool) error

	// SetDisappearingTimer define o tempo de expiração das mensagens.
	SetDisappearingTimer(ctx context.Context, txtID string, group domain.JID, d time.Duration, at time.Time) error

	// UpdateGroupParticipants adiciona ou remove participantes.
	UpdateGroupParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.ParticipantAction) (any, error)
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
