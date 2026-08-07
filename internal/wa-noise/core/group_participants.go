package wanoise

import (
	"context"

	"wa-api/internal/wa-noise/capabilities/group"
	"wa-api/internal/wa-noise/protocol/types"
)

// Fachada; ver o cabecalho de group.go.

// ParticipantChange e' apelido de tipo, e as quatro constantes sao os MESMOS
// valores de group.Change* — nao copias. Chamadores externos que passam
// wa-noise.ParticipantChangeAdd continuam compilando sem conversao.
type ParticipantChange = group.ParticipantChange

const (
	ParticipantChangeAdd     ParticipantChange = group.ChangeAdd
	ParticipantChangeRemove  ParticipantChange = group.ChangeRemove
	ParticipantChangePromote ParticipantChange = group.ChangePromote
	ParticipantChangeDemote  ParticipantChange = group.ChangeDemote
)

// UpdateGroupParticipants can be used to add, remove, promote and demote members in a WhatsApp group.
func (cli *Client) UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action ParticipantChange) ([]types.GroupParticipant, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.UpdateParticipants(ctx, cli.groupT(), jid, participantChanges, action)
}

// GetGroupRequestParticipants gets the list of participants that have requested to join the group.
func (cli *Client) GetGroupRequestParticipants(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.GetRequestParticipants(ctx, cli.groupT(), jid)
}

// ParticipantRequestChange e' apelido de tipo; ver ParticipantChange.
type ParticipantRequestChange = group.ParticipantRequestChange

const (
	ParticipantChangeApprove ParticipantRequestChange = group.RequestApprove
	ParticipantChangeReject  ParticipantRequestChange = group.RequestReject
)

// UpdateGroupRequestParticipants can be used to approve or reject requests to join the group.
func (cli *Client) UpdateGroupRequestParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action ParticipantRequestChange) ([]types.GroupParticipant, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	return group.UpdateRequestParticipants(ctx, cli.groupT(), jid, participantChanges, action)
}
