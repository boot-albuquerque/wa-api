package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ChatPresenceUseCase sets chat presence (typing/recording)
type ChatPresenceUseCase struct {
	presence appport.PresenceController
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewChatPresenceUseCase creates a new instance
func NewChatPresenceUseCase(pc appport.PresenceController, jr appport.JIDResolver, logger appport.Logger) *ChatPresenceUseCase {
	return &ChatPresenceUseCase{presence: pc, jids: jr, logger: logger}
}

// Execute sets chat presence
func (uc *ChatPresenceUseCase) Execute(ctx context.Context, userID string, req domain.ChatPresenceRequest) error {
	if err := uc.presence.EnsureSession(ctx, userID); err != nil {
		return fmt.Errorf("no session")
	}

	if len(req.Phone) < 1 {
		return fmt.Errorf("missing Phone in Payload")
	}

	if len(req.State) < 1 {
		return fmt.Errorf("missing State in Payload")
	}

	jid, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		return fmt.Errorf("could not parse Phone")
	}

	if err := uc.presence.SendChatPresence(ctx, userID, jid, req.State, req.Media); err != nil {
		uc.logger.Error(ctx, "Failed to send chat presence", "error", err, "user_id", userID, "phone", req.Phone)
		return fmt.Errorf("failure sending chat presence to Whatsapp servers")
	}

	return nil
}
