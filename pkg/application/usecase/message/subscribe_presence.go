package message

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SubscribePresenceUseCase subscribes to contact presence updates
type SubscribePresenceUseCase struct {
	presence appport.PresenceSubscriber
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewSubscribePresenceUseCase creates a new instance
func NewSubscribePresenceUseCase(pc appport.PresenceSubscriber, jr appport.JIDResolver, logger appport.Logger) *SubscribePresenceUseCase {
	return &SubscribePresenceUseCase{presence: pc, jids: jr, logger: logger}
}

// Execute subscribes to a contact's presence
func (uc *SubscribePresenceUseCase) Execute(ctx context.Context, userID string, req domain.SubscribePresenceRequest) error {
	if err := uc.presence.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return err
	}

	if len(req.Phone) < 1 {
		return apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in Payload", false, nil)
	}

	jid, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		return apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	if err := uc.presence.SubscribePresence(ctx, userID, jid); err != nil {
		uc.logger.Error(ctx, "Failed to subscribe to presence", "error", err, "user_id", userID, "phone", req.Phone)
		return fmt.Errorf("failure subscribing to presence")
	}

	uc.logger.Info(ctx, "Subscribed to presence", "jid", string(jid), "user_id", userID)
	return nil
}
