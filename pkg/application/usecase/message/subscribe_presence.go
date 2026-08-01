package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/whatsmeow"
)

// SubscribePresenceUseCase subscribes to contact presence updates
type SubscribePresenceUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSubscribePresenceUseCase creates a new instance
func NewSubscribePresenceUseCase(cp appport.ClientProvider, logger appport.Logger) *SubscribePresenceUseCase {
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
		uc.logger.Error(ctx, "Failed to subscribe to presence", "error", err, "user_id", userID, "phone", req.Phone)
		return fmt.Errorf("failure subscribing to presence")
	}

	uc.logger.Info(ctx, "Subscribed to presence", "jid", jid.String(), "user_id", userID)
	return nil
}
