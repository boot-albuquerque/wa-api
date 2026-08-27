package group

import (
	"context"
	"fmt"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	"wa-api/pkg/domain"

	wa "wa-api/internal/wa-noise"
)

// UpdateGroupParticipants adiciona ou remove participantes.
func (a *GroupAdapter) UpdateGroupParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.ParticipantAction) (domain.ParticipantsUpdate, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	jids, err := wajid.ToJIDs(participants)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}

	// O upstream tratava qualquer ação diferente de "add" como remoção;
	// a validação do valor agora é do use case, e aqui só resta o mapeamento.
	change := wa.ParticipantChangeRemove
	if action == domain.ParticipantAdd {
		change = wa.ParticipantChangeAdd
	}
	res, err := client.UpdateGroupParticipants(ctx, jid, jids, change)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	// Confirmado: o protocolo devolve a lista resultante na mesma resposta, e é
	// ela que volta aqui. Este transporte lê a pós-condição na própria chamada.
	out := make([]domain.GroupParticipant, 0, len(res))
	for _, p := range res {
		out = append(out, toDomainGroupParticipant(p))
	}
	return domain.ParticipantsUpdate{Participants: out, Confirmed: true}, nil
}

// GetRequestParticipants lista quem solicitou entrar no grupo.
func (a *GroupAdapter) GetRequestParticipants(ctx context.Context, txtID string, group domain.JID) ([]domain.GroupJoinRequest, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := wajid.ToJID(group)
	if err != nil {
		return nil, err
	}
	res, err := client.GetGroupRequestParticipants(ctx, jid)
	if err != nil {
		return nil, err
	}
	// This transport reports the requester and the moment; who tried to add
	// them, and by which method, is not in the protocol answer and stays zero.
	out := make([]domain.GroupJoinRequest, 0, len(res))
	for _, r := range res {
		out = append(out, domain.GroupJoinRequest{
			JID:         jidOrEmpty(r.JID),
			RequestedAt: r.RequestedAt,
		})
	}
	return out, nil
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
