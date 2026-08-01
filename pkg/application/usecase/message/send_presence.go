package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow/types"
)

// SendPresenceUseCase sets global presence status
type SendPresenceUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendPresenceUseCase creates a new instance
func NewSendPresenceUseCase(cp appport.ClientProvider, logger appport.Logger) *SendPresenceUseCase {
	return &SendPresenceUseCase{clientProvider: cp, logger: logger}
}

// Execute sets presence status
func (uc *SendPresenceUseCase) Execute(ctx context.Context, userID string, req domain.SendPresenceRequest) error {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return fmt.Errorf("no session")
	}

	var presence types.Presence
	switch req.Type {
	case "available":
		presence = types.PresenceAvailable
	case "unavailable":
		presence = types.PresenceUnavailable
	default:
		return fmt.Errorf("invalid presence type. Allowed values: 'available', 'unavailable'")
	}

	uc.logger.Info(ctx, "Setting presence", "presence", req.Type, "user_id", userID)

	if err := client.SendPresence(ctx, presence); err != nil {
		uc.logger.Error(ctx, "Failed to send presence", "error", err, "user_id", userID)
		return fmt.Errorf("failure sending presence to Whatsapp servers")
	}

	return nil
}
