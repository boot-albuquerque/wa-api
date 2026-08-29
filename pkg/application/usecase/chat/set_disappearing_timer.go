package chat

import (
	"context"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
)

// SetDisappearingTimerUseCase sets the disappearing message timer for a
// specific chat (private or group).
type SetDisappearingTimerUseCase struct {
	chats  appport.ChatOperations
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewSetDisappearingTimerUseCase creates a new instance.
func NewSetDisappearingTimerUseCase(co appport.ChatOperations, jr appport.JIDResolver, l appport.Logger) *SetDisappearingTimerUseCase {
	return &SetDisappearingTimerUseCase{chats: co, jids: jr, logger: l}
}

// Execute sets the disappearing timer for the given chat.
// duration is validated against the discrete set the WhatsApp server
// accepts: off, 24h, 7d, 90d.
func (uc *SetDisappearingTimerUseCase) Execute(ctx context.Context, txtID, chatJID, duration string) error {
	if err := uc.chats.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return err
	}

	jid, err := uc.jids.ResolveJID(ctx, chatJID)
	if err != nil {
		uc.logger.Error(ctx, "could not parse chat JID", "jid", chatJID, "error", err)
		return apperr.New("invalid_jid", apperr.CategoryValidation,
			"could not parse chat JID", false, err)
	}

	d, ok := parseDisappearingDuration(duration)
	if !ok {
		uc.logger.Warn(ctx, "invalid disappearing timer duration", "txtID", txtID, "chatJID", chatJID, "duration", duration)
		return apperr.New("invalid_duration", apperr.CategoryValidation,
			"duration must be one of: 0, 24h, 7d, 90d", false, nil)
	}

	if err := uc.chats.SetDisappearingTimer(ctx, txtID, jid, d, time.Now()); err != nil {
		uc.logger.Error(ctx, "failed to set disappearing timer", "txtID", txtID, "chatJID", chatJID, "duration", duration, "error", err)
		return err
	}

	return nil
}
