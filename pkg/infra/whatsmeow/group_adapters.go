package whatsmeow

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	wa "go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// toJIDs converte uma lista de domain.JID para o tipo do SDK.
func toJIDs(in []domain.JID) ([]types.JID, error) {
	out := make([]types.JID, len(in))
	for i, j := range in {
		parsed, err := toJID(j)
		if err != nil {
			return nil, err
		}
		out[i] = parsed
	}
	return out, nil
}

// GroupAdapter implementa as quatro portas de grupo sobre o clientManager.
//
// Um tipo só implementando GroupDirectory, GroupLifecycle, GroupSettings e
// GroupRequests: a segmentação existe para que cada use case dependa apenas
// da capacidade que exerce, não para multiplicar adapters — todos falam com
// o mesmo cliente.
type GroupAdapter struct {
	*SessionGuardAdapter
}

// NewGroupAdapter cria o adapter com a função de lookup.
func NewGroupAdapter(getClient func(txtID string) *wa.Client) *GroupAdapter {
	return &GroupAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

// bgCtx é context.Background(), nomeado para deixar explícito que não é
// esquecimento: GroupManagementUseCase chamava estas operações com
// context.Background(), ignorando o cancelamento da requisição. O
// comportamento foi preservado literalmente — trocá-lo por ctx é correção de
// lógica, não movimento, e passaria a abortar operações de escrita em grupo
// quando o cliente HTTP desiste. Fica como follow-up nomeado.
var bgCtx = context.Background()

// client devolve o cliente da sessão ou o erro tipado de sessão ausente.
func (a *GroupAdapter) client(txtID string) (*wa.Client, error) {
	client := a.getClient(txtID)
	if client == nil {
		return nil, ErrNoSession(txtID, nil)
	}
	return client, nil
}

// GetGroupInfo devolve os metadados de um grupo.
func (a *GroupAdapter) GetGroupInfo(ctx context.Context, txtID string, group domain.JID) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := toJID(group)
	if err != nil {
		return nil, err
	}
	return client.GetGroupInfo(ctx, jid)
}

// GetGroupInfoFromLink devolve os metadados a partir de um código de convite.
func (a *GroupAdapter) GetGroupInfoFromLink(ctx context.Context, txtID, code string) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	return client.GetGroupInfoFromLink(ctx, code)
}

// GetGroupInviteLink devolve o link de convite de um grupo.
func (a *GroupAdapter) GetGroupInviteLink(ctx context.Context, txtID string, group domain.JID) (string, error) {
	client, err := a.client(txtID)
	if err != nil {
		return "", err
	}
	jid, err := toJID(group)
	if err != nil {
		return "", err
	}
	return client.GetGroupInviteLink(ctx, jid, false)
}

// ListJoinedGroups devolve os grupos de que a sessão participa e a contagem.
func (a *GroupAdapter) ListJoinedGroups(ctx context.Context, txtID string) (any, int, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, 0, err
	}
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, 0, err
	}
	return groups, len(groups), nil
}

// CreateGroup cria um grupo com os participantes informados.
func (a *GroupAdapter) CreateGroup(ctx context.Context, txtID, name string, participants []domain.JID) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	jids, err := toJIDs(participants)
	if err != nil {
		return nil, err
	}
	return client.CreateGroup(ctx, wa.ReqCreateGroup{Name: name, Participants: jids})
}

// JoinGroup entra num grupo por código de convite.
func (a *GroupAdapter) JoinGroup(ctx context.Context, txtID, code string) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	return client.JoinGroupWithLink(bgCtx, code)
}

// LeaveGroup sai de um grupo.
func (a *GroupAdapter) LeaveGroup(ctx context.Context, txtID string, group domain.JID) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	return client.LeaveGroup(bgCtx, jid)
}

// SetGroupName renomeia o grupo.
func (a *GroupAdapter) SetGroupName(ctx context.Context, txtID string, group domain.JID, name string) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupName(bgCtx, jid, name)
}

// SetGroupTopic define a descrição do grupo.
func (a *GroupAdapter) SetGroupTopic(ctx context.Context, txtID string, group domain.JID, topic string) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupTopic(bgCtx, jid, "", "", topic)
}

// SetGroupPhoto define a foto do grupo; photo nil remove a foto.
func (a *GroupAdapter) SetGroupPhoto(ctx context.Context, txtID string, group domain.JID, photo []byte) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	_, err = client.SetGroupPhoto(bgCtx, jid, photo)
	return err
}

// SetGroupAnnounce liga/desliga o modo somente-administradores.
func (a *GroupAdapter) SetGroupAnnounce(ctx context.Context, txtID string, group domain.JID, announce bool) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupAnnounce(bgCtx, jid, announce)
}

// SetGroupLocked tranca/destranca as configurações do grupo.
func (a *GroupAdapter) SetGroupLocked(ctx context.Context, txtID string, group domain.JID, locked bool) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupLocked(bgCtx, jid, locked)
}

// SetDisappearingTimer define o tempo de expiração das mensagens.
func (a *GroupAdapter) SetDisappearingTimer(ctx context.Context, txtID string, group domain.JID, d time.Duration, at time.Time) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	return client.SetDisappearingTimer(bgCtx, jid, d, at)
}

// UpdateGroupParticipants adiciona ou remove participantes.
func (a *GroupAdapter) UpdateGroupParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.ParticipantAction) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := toJID(group)
	if err != nil {
		return nil, err
	}
	jids, err := toJIDs(participants)
	if err != nil {
		return nil, err
	}

	// O upstream tratava qualquer ação diferente de "add" como remoção;
	// a validação do valor agora é do use case, e aqui só resta o mapeamento.
	change := wa.ParticipantChangeRemove
	if action == domain.ParticipantAdd {
		change = wa.ParticipantChangeAdd
	}
	return client.UpdateGroupParticipants(ctx, jid, jids, change)
}

// GetRequestParticipants lista quem solicitou entrar no grupo.
func (a *GroupAdapter) GetRequestParticipants(ctx context.Context, txtID string, group domain.JID) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := toJID(group)
	if err != nil {
		return nil, err
	}
	return client.GetGroupRequestParticipants(ctx, jid)
}

// UpdateRequestParticipants aprova ou rejeita solicitações de entrada.
func (a *GroupAdapter) UpdateRequestParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	jids, err := toJIDs(participants)
	if err != nil {
		return err
	}

	var change wa.ParticipantRequestChange
	switch action {
	case domain.RequestApprove:
		change = wa.ParticipantChangeApprove
	case domain.RequestReject:
		change = wa.ParticipantChangeReject
	default:
		return fmt.Errorf("whatsmeow: unknown request action %q", string(action))
	}

	_, err = client.UpdateGroupRequestParticipants(ctx, jid, jids, change)
	return err
}

// SetJoinApprovalMode liga/desliga a exigência de aprovação para entrar.
func (a *GroupAdapter) SetJoinApprovalMode(ctx context.Context, txtID string, group domain.JID, mode bool) error {
	client, err := a.client(txtID)
	if err != nil {
		return err
	}
	jid, err := toJID(group)
	if err != nil {
		return err
	}
	return client.SetGroupJoinApprovalMode(ctx, jid, mode)
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.GroupDirectory = (*GroupAdapter)(nil)
	_ appport.GroupLifecycle = (*GroupAdapter)(nil)
	_ appport.GroupSettings  = (*GroupAdapter)(nil)
	_ appport.GroupRequests  = (*GroupAdapter)(nil)
)
