package usecase

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	appport "disparazap/internal/application/port"
	"disparazap/internal/shared/domain"
	"disparazap/internal/infra/whatsmeow"
)

// SubscribePresenceUseCase subscribes to contact presence updates
type SubscribePresenceUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewSubscribePresenceUseCase creates a new instance
func NewSubscribePresenceUseCase(cp appport.ClientProvider, logger zerolog.Logger) *SubscribePresenceUseCase {
	return &SubscribePresenceUseCase{clientProvider: cp, logger: logger}
}

// Execute subscribes to a contact's presence
func (uc *SubscribePresenceUseCase) Execute(ctx context.Context, userID string, req domain.SubscribePresenceRequest) error {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return fmt.Errorf("no session")
	}

	if len(req.Phone) < 1 {
		return fmt.Errorf("missing Phone in Payload")
	}

	jid, ok := whatsmeow.ParseJID(req.Phone)
	if !ok {
		return fmt.Errorf("could not parse Phone")
	}

	if err := client.SubscribePresence(ctx, jid); err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Str("phone", req.Phone).Msg("Failed to subscribe to presence")
		return fmt.Errorf("failure subscribing to presence")
	}

	uc.logger.Info().Str("jid", jid.String()).Str("user_id", userID).Msg("Subscribed to presence")
	return nil
}
