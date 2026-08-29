package chat

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// RejectCallUseCase rejects an incoming call
type RejectCallUseCase struct {
	chats  appport.CallRejecter
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewRejectCallUseCase creates a new instance
func NewRejectCallUseCase(co appport.CallRejecter, jr appport.JIDResolver, logger appport.Logger) *RejectCallUseCase {
	return &RejectCallUseCase{chats: co, jids: jr, logger: logger}
}

// Execute rejects a call
func (uc *RejectCallUseCase) Execute(ctx context.Context, userID string, req domain.RejectCallRequest) (*domain.RejectCallResult, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.CallFrom == "" {
		return nil, apperr.New("missing_call_from", apperr.CategoryValidation, "missing call_from in Payload", false, nil)
	}

	if req.CallID == "" {
		return nil, apperr.New("missing_call_id", apperr.CategoryValidation, "missing call_id in Payload", false, nil)
	}

	callFrom, err := uc.jids.ResolveQualifiedJID(ctx, req.CallFrom)
	if err != nil {
		return nil, apperr.New("invalid_call_from", apperr.CategoryValidation, "could not parse call_from", false, nil)
	}

	if err := uc.chats.RejectCall(ctx, userID, callFrom, req.CallID); err != nil {
		uc.logger.Error(ctx, "failed to reject call", "error", err, "user_id", userID)
		return nil, fmt.Errorf("error rejecting call: %w", err)
	}

	uc.logger.Info(ctx, "Call rejected", "call_id", req.CallID, "call_from", req.CallFrom)

	return &domain.RejectCallResult{
		Details: "Call rejected",
		CallID:  req.CallID,
	}, nil
}
