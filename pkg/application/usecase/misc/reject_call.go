package misc

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow/types"
)

// RejectCallUseCase rejects an incoming call
type RejectCallUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewRejectCallUseCase creates a new instance
func NewRejectCallUseCase(cp appport.ClientProvider, logger appport.Logger) *RejectCallUseCase {
	return &RejectCallUseCase{clientProvider: cp, logger: logger}
}

// Execute rejects a call
func (uc *RejectCallUseCase) Execute(ctx context.Context, userID string, req domain.RejectCallRequest) (*domain.RejectCallResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	if req.CallFrom == "" {
		return nil, fmt.Errorf("missing call_from in Payload")
	}

	if req.CallID == "" {
		return nil, fmt.Errorf("missing call_id in Payload")
	}

	callFrom, err := types.ParseJID(req.CallFrom)
	if err != nil {
		return nil, fmt.Errorf("could not parse call_from")
	}

	err = client.RejectCall(ctx, callFrom, req.CallID)
	if err != nil {
		uc.logger.Error(ctx, "failed to reject call", "error", err, "user_id", userID)
		return nil, fmt.Errorf("error rejecting call: %w", err)
	}

	uc.logger.Info(ctx, "Call rejected", "call_id", req.CallID, "call_from", req.CallFrom)

	return &domain.RejectCallResult{
		Details: "Call rejected",
		CallID:  req.CallID,
	}, nil
}
