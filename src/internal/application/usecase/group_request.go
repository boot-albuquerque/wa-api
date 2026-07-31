package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// GroupRequestUseCase encapsula a lógica de gerenciamento de solicitações de entrada em grupos
type GroupRequestUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewGroupRequestUseCase cria uma nova instância
func NewGroupRequestUseCase(cp appport.ClientProvider, l zerolog.Logger) *GroupRequestUseCase {
	return &GroupRequestUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// ExecuteGetGroupRequestParticipants lista os participantes que solicitaram entrar
func (uc *GroupRequestUseCase) ExecuteGetGroupRequestParticipants(ctx context.Context, userID string, req domain.GetGroupRequestParticipantsRequest) (json.RawMessage, error) {
	if req.GroupJID == "" {
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to get whatsmeow client")
		return nil, fmt.Errorf("no session")
	}

	group, ok := parseJID(req.GroupJID)
	if !ok {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	resp, err := client.GetGroupRequestParticipants(ctx, group)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Str("group_jid", req.GroupJID).Msg("failed to get group request participants")
		return nil, fmt.Errorf("failed to get group request participants: %w", err)
	}

	responseJson, err := json.Marshal(resp)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to marshal response")
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return responseJson, nil
}

// ExecuteUpdateGroupRequestParticipants aprova ou rejeita solicitações de entrada
func (uc *GroupRequestUseCase) ExecuteUpdateGroupRequestParticipants(ctx context.Context, userID string, req domain.UpdateGroupRequestParticipantsRequest) (*domain.UpdateGroupRequestParticipantsResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to get whatsmeow client")
		return nil, fmt.Errorf("no session")
	}

	// Validate request
	if req.GroupJID == "" {
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	if len(req.Phone) < 1 {
		return nil, fmt.Errorf("missing Phone in payload")
	}

	if req.Action == "" {
		return nil, fmt.Errorf("missing Action in payload")
	}

	group, ok := parseJID(req.GroupJID)
	if !ok {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	// Parse phone numbers
	phoneParsed := make([]types.JID, len(req.Phone))
	for i, phone := range req.Phone {
		phoneParsed[i], ok = parseJID(phone)
		if !ok {
			return nil, fmt.Errorf("could not parse Phone")
		}
	}

	// Parse action
	var action whatsmeow.ParticipantRequestChange
	switch req.Action {
	case "approve":
		action = whatsmeow.ParticipantChangeApprove
	case "reject":
		action = whatsmeow.ParticipantChangeReject
	default:
		return nil, fmt.Errorf("invalid Action in payload (must be approve or reject)")
	}

	_, err = client.UpdateGroupRequestParticipants(ctx, group, phoneParsed, action)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Str("group_jid", req.GroupJID).Str("action", req.Action).Msg("failed to update group request participants")
		return nil, fmt.Errorf("failed to update group request participants: %w", err)
	}

	return &domain.UpdateGroupRequestParticipantsResult{
		Details: "Group request participants updated successfully",
	}, nil
}

// ExecuteSetGroupJoinApprovalMode alterna o requisito de aprovação para entrar no grupo
func (uc *GroupRequestUseCase) ExecuteSetGroupJoinApprovalMode(ctx context.Context, userID string, req domain.SetGroupJoinApprovalModeRequest) (*domain.SetGroupJoinApprovalModeResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to get whatsmeow client")
		return nil, fmt.Errorf("no session")
	}

	if req.GroupJID == "" {
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	group, ok := parseJID(req.GroupJID)
	if !ok {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	err = client.SetGroupJoinApprovalMode(ctx, group, req.Mode)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Str("group_jid", req.GroupJID).Bool("mode", req.Mode).Msg("failed to set group join approval mode")
		return nil, fmt.Errorf("failed to set group join approval mode: %w", err)
	}

	return &domain.SetGroupJoinApprovalModeResult{
		Details: "Group join approval mode updated successfully",
	}, nil
}

// parseJID é um helper que converte string para types.JID
// Duplicado aqui por necessidade (também existe em handlers_grouprequests.go)
func parseJID(jidStr string) (types.JID, bool) {
	jid, err := types.ParseJID(jidStr)
	return jid, err == nil
}
