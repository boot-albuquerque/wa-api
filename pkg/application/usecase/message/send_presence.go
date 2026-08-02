package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendPresenceUseCase sets global presence status
type SendPresenceUseCase struct {
	presence appport.PresenceController
	logger   appport.Logger
}

// NewSendPresenceUseCase creates a new instance
func NewSendPresenceUseCase(pc appport.PresenceController, logger appport.Logger) *SendPresenceUseCase {
	return &SendPresenceUseCase{presence: pc, logger: logger}
}

// Execute sets presence status
func (uc *SendPresenceUseCase) Execute(ctx context.Context, userID string, req domain.SendPresenceRequest) error {
	if err := uc.presence.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return err
	}

	var presence domain.PresenceType
	switch req.Type {
	case "available":
		presence = domain.PresenceAvailable
	case "unavailable":
		presence = domain.PresenceUnavailable
	default:
		return fmt.Errorf("invalid presence type. Allowed values: 'available', 'unavailable'")
	}

	uc.logger.Info(ctx, "Setting presence", "presence", req.Type, "user_id", userID)

	if err := uc.presence.SendPresence(ctx, userID, presence); err != nil {
		uc.logger.Error(ctx, "Failed to send presence", "error", err, "user_id", userID)
		return fmt.Errorf("failure sending presence to Whatsapp servers")
	}

	return nil
}
