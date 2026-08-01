package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/whatsmeow"

	"go.mau.fi/whatsmeow/types"
)

// ChatPresenceUseCase sets chat presence (typing/recording)
type ChatPresenceUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewChatPresenceUseCase creates a new instance
func NewChatPresenceUseCase(cp appport.ClientProvider, logger appport.Logger) *ChatPresenceUseCase {
	return &ChatPresenceUseCase{clientProvider: cp, logger: logger}
}

// Execute sets chat presence
func (uc *ChatPresenceUseCase) Execute(ctx context.Context, userID string, req domain.ChatPresenceRequest) error {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return fmt.Errorf("no session")
	}

	if len(req.Phone) < 1 {
		return fmt.Errorf("missing Phone in Payload")
	}

	if len(req.State) < 1 {
		return fmt.Errorf("missing State in Payload")
	}

	jid, ok := whatsmeow.ParseJID(req.Phone)
	if !ok {
		return fmt.Errorf("could not parse Phone")
	}

	err = client.SendChatPresence(ctx, jid, types.ChatPresence(req.State), types.ChatPresenceMedia(req.Media))
	if err != nil {
		uc.logger.Error(ctx, "Failed to send chat presence", "error", err, "user_id", userID, "phone", req.Phone)
		return fmt.Errorf("failure sending chat presence to Whatsapp servers")
	}

	return nil
}
