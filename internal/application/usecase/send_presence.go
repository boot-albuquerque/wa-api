package usecase

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
	appport "wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// SendPresenceUseCase sets global presence status
type SendPresenceUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewSendPresenceUseCase creates a new instance
func NewSendPresenceUseCase(cp appport.ClientProvider, logger zerolog.Logger) *SendPresenceUseCase {
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

	uc.logger.Info().Str("presence", req.Type).Str("user_id", userID).Msg("Setting presence")

	if err := client.SendPresence(ctx, presence); err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to send presence")
		return fmt.Errorf("failure sending presence to Whatsapp servers")
	}

	return nil
}
