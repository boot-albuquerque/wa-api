package chat

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// RejectCallUseCase rejects an incoming call
type RejectCallUseCase struct {
	chats  appport.ChatOperations
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewRejectCallUseCase creates a new instance
func NewRejectCallUseCase(co appport.ChatOperations, jr appport.JIDResolver, logger appport.Logger) *RejectCallUseCase {
	return &RejectCallUseCase{chats: co, jids: jr, logger: logger}
}

// Execute rejects a call
func (uc *RejectCallUseCase) Execute(ctx context.Context, userID string, req domain.RejectCallRequest) (*domain.RejectCallResult, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.CallFrom == "" {
		return nil, fmt.Errorf("missing call_from in Payload")
	}

	if req.CallID == "" {
		return nil, fmt.Errorf("missing call_id in Payload")
	}

	callFrom, err := uc.jids.ResolveQualifiedJID(ctx, req.CallFrom)
	if err != nil {
		return nil, fmt.Errorf("could not parse call_from")
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
