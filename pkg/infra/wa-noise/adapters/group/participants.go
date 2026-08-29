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

	// F263: o upstream tratava qualquer ação diferente de "add" como remoção,
	// o que silenciosamente enviava "remove" para "promote"/"demote" antes de o
	// use case sequer aceitar esses valores. A validação do valor é do use case
	// (group_management.go); aqui, com só quatro valores possíveis, um switch
	// exaustivo é o que impede a mesma armadilha de voltar quando um quinto
	// action for aceite lá sem entrar aqui — o default devolve erro em vez de
	// silenciosamente cair em "remove".
	var change wa.ParticipantChange
	switch action {
	case domain.ParticipantAdd:
		change = wa.ParticipantChangeAdd
	case domain.ParticipantRemove:
		change = wa.ParticipantChangeRemove
	case domain.ParticipantPromote:
		change = wa.ParticipantChangePromote
	case domain.ParticipantDemote:
		change = wa.ParticipantChangeDemote
	default:
		return domain.ParticipantsUpdate{}, fmt.Errorf("wanoise: unknown participant action %q", string(action))
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
//
// F280: `client.UpdateGroupRequestParticipants` devolve, por solicitante, o
// JID resolvido e um `Error` diferente de zero quando aquele solicitante
// falhou — a mesma forma que `UpdateGroupParticipants` (acima) já lê e
// devolve para add/remove. Até 2026-08-28 esta função descartava o
// resultado (`_, err := …`) e a rota respondia uma frase fixa; um sucesso
// parcial (aprovar três, um falhar) era indistinguível de sucesso total.
func (a *GroupAdapter) UpdateRequestParticipants(ctx context.Context, txtID string, group domain.JID, participants []domain.JID, action domain.RequestAction) (domain.ParticipantsUpdate, error) {
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

	var change wa.ParticipantRequestChange
	switch action {
	case domain.RequestApprove:
		change = wa.ParticipantChangeApprove
	case domain.RequestReject:
		change = wa.ParticipantChangeReject
	default:
		return domain.ParticipantsUpdate{}, fmt.Errorf("wanoise: unknown request action %q", string(action))
	}

	res, err := client.UpdateGroupRequestParticipants(ctx, jid, jids, change)
	if err != nil {
		return domain.ParticipantsUpdate{}, err
	}
	out := make([]domain.GroupParticipant, 0, len(res))
	for _, p := range res {
		out = append(out, toDomainGroupParticipant(p))
	}
	return domain.ParticipantsUpdate{Participants: out, Confirmed: true}, nil
}
