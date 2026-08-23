package message

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ChatPresenceUseCase sets chat presence (typing/recording)
type ChatPresenceUseCase struct {
	presence appport.PresenceAnnouncer
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewChatPresenceUseCase creates a new instance
func NewChatPresenceUseCase(pc appport.PresenceAnnouncer, jr appport.JIDResolver, logger appport.Logger) *ChatPresenceUseCase {
	return &ChatPresenceUseCase{presence: pc, jids: jr, logger: logger}
}

// Execute sets chat presence
func (uc *ChatPresenceUseCase) Execute(ctx context.Context, userID string, req domain.ChatPresenceRequest) error {
	if err := uc.presence.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return err
	}

	if len(req.Phone) < 1 {
		return apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in Payload", false, nil)
	}

	if len(req.State) < 1 {
		return apperr.New("missing_state", apperr.CategoryValidation, "missing State in Payload", false, nil)
	}

	jid, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		return apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	if err := uc.presence.SendChatPresence(ctx, userID, jid, req.State, req.Media); err != nil {
		uc.logger.Error(ctx, "Failed to send chat presence", "error", err, "user_id", userID, "phone", req.Phone)
		return fmt.Errorf("failure sending chat presence to Whatsapp servers")
	}

	return nil
}
