package group

import (
	"context"
	"fmt"
	wajid "wa-api/pkg/infra/wa-noise/jid"

	"wa-api/pkg/domain"

	wa "wa-api/internal/wa-noise"
)

// UpdateGroupParticipants adiciona ou remove participantes.
func (a *GroupAdapter) UpdateGroupParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.ParticipantAction) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return nil, err
	}
	jids, err := wajid.ToJIDs(participants)
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
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return nil, err
	}
	return client.GetGroupRequestParticipants(ctx, jid)
}

// UpdateRequestParticipants aprova ou rejeita solicitações de entrada.
func (a *GroupAdapter) UpdateRequestParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return err
	}
	jids, err := wajid.ToJIDs(participants)
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
		return fmt.Errorf("wanoise: unknown request action %q", string(action))
	}

	_, err = client.UpdateGroupRequestParticipants(ctx, jid, jids, change)
	return err
}
