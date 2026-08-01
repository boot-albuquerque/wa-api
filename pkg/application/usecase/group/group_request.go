package group

import (
	"context"
	"encoding/json"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GroupRequestUseCase encapsula a lógica de gerenciamento de solicitações de entrada em grupos
type GroupRequestUseCase struct {
	requests appport.GroupRequests
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewGroupRequestUseCase cria uma nova instância
func NewGroupRequestUseCase(gr appport.GroupRequests, jr appport.JIDResolver, l appport.Logger) *GroupRequestUseCase {
	return &GroupRequestUseCase{
		requests: gr,
		jids:     jr,
		logger:   l,
	}
}

// ExecuteGetGroupRequestParticipants lista os participantes que solicitaram entrar
func (uc *GroupRequestUseCase) ExecuteGetGroupRequestParticipants(ctx context.Context, userID string, req domain.GetGroupRequestParticipantsRequest) (json.RawMessage, error) {
	if req.GroupJID == "" {
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	if err := uc.requests.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, fmt.Errorf("no session")
	}

	group, err := uc.jids.ResolveQualifiedJID(ctx, req.GroupJID)
	if err != nil {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	resp, err := uc.requests.GetRequestParticipants(ctx, userID, group)
	if err != nil {
		uc.logger.Error(ctx, "failed to get group request participants", "error", err, "user_id", userID, "group_jid", req.GroupJID)
		return nil, fmt.Errorf("failed to get group request participants: %w", err)
	}

	responseJson, err := json.Marshal(resp)
	if err != nil {
		uc.logger.Error(ctx, "failed to marshal response", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return responseJson, nil
}

// ExecuteUpdateGroupRequestParticipants aprova ou rejeita solicitações de entrada
func (uc *GroupRequestUseCase) ExecuteUpdateGroupRequestParticipants(ctx context.Context, userID string, req domain.UpdateGroupRequestParticipantsRequest) (*domain.UpdateGroupRequestParticipantsResult, error) {
	if err := uc.requests.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
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

	group, err := uc.jids.ResolveQualifiedJID(ctx, req.GroupJID)
	if err != nil {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	// Parse phone numbers
	phoneParsed := make([]domain.JID, len(req.Phone))
	for i, phone := range req.Phone {
		phoneParsed[i], err = uc.jids.ResolveQualifiedJID(ctx, phone)
		if err != nil {
			return nil, fmt.Errorf("could not parse Phone")
		}
	}

	// Parse action
	var action domain.RequestAction
	switch req.Action {
	case "approve":
		action = domain.RequestApprove
	case "reject":
		action = domain.RequestReject
	default:
		return nil, fmt.Errorf("invalid Action in payload (must be approve or reject)")
	}

	if err := uc.requests.UpdateRequestParticipants(ctx, userID, group, phoneParsed, action); err != nil {
		uc.logger.Error(ctx, "failed to update group request participants", "error", err, "user_id", userID, "group_jid", req.GroupJID, "action", req.Action)
		return nil, fmt.Errorf("failed to update group request participants: %w", err)
	}

	return &domain.UpdateGroupRequestParticipantsResult{
		Details: "Group request participants updated successfully",
	}, nil
}

// ExecuteSetGroupJoinApprovalMode alterna o requisito de aprovação para entrar no grupo
func (uc *GroupRequestUseCase) ExecuteSetGroupJoinApprovalMode(ctx context.Context, userID string, req domain.SetGroupJoinApprovalModeRequest) (*domain.SetGroupJoinApprovalModeResult, error) {
	if err := uc.requests.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, fmt.Errorf("no session")
	}

	if req.GroupJID == "" {
		return nil, fmt.Errorf("missing groupJID parameter")
	}

	group, err := uc.jids.ResolveQualifiedJID(ctx, req.GroupJID)
	if err != nil {
		return nil, fmt.Errorf("could not parse Group JID")
	}

	if err := uc.requests.SetJoinApprovalMode(ctx, userID, group, req.Mode); err != nil {
		uc.logger.Error(ctx, "failed to set group join approval mode", "error", err, "user_id", userID, "group_jid", req.GroupJID, "mode", req.Mode)
		return nil, fmt.Errorf("failed to set group join approval mode: %w", err)
	}

	return &domain.SetGroupJoinApprovalModeResult{
		Details: "Group join approval mode updated successfully",
	}, nil
}
