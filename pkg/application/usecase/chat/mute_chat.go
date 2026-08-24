package chat

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// Allowed mute durations. WhatsApp offers exactly three discrete options:
// 8 hours, 1 week, and forever (represented as zero duration to BuildMute).
var allowedMuteDurations = map[time.Duration]bool{
	8 * time.Hour:      true,
	7 * 24 * time.Hour: true,
	0:                  true, // forever
}

// MuteChatUseCase mutes or unmutes a chat.
type MuteChatUseCase struct {
	chats  appport.ChatMuter
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewMuteChatUseCase creates a new instance.
func NewMuteChatUseCase(cm appport.ChatMuter, jr appport.JIDResolver, logger appport.Logger) *MuteChatUseCase {
	return &MuteChatUseCase{chats: cm, jids: jr, logger: logger}
}

// Execute mutes or unmutes a chat.
func (uc *MuteChatUseCase) Execute(ctx context.Context, userID string, req domain.MuteChatRequest) (*domain.MuteChatResult, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.Jid == "" {
		return nil, apperr.New("missing_jid", apperr.CategoryValidation, "missing jid in payload", false, nil)
	}

	chatJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Jid)
	if err != nil {
		return nil, apperr.New("invalid_chat_jid", apperr.CategoryValidation, "invalid chat JID format", false, nil)
	}

	var muteDuration time.Duration
	if req.Mute {
		if req.MuteDuration != nil {
			muteDuration = *req.MuteDuration
		}
		if !allowedMuteDurations[muteDuration] {
			return nil, apperr.New("invalid_mute_duration", apperr.CategoryValidation,
				fmt.Sprintf("mute_duration must be 8h, 168h (1 week), or omitted (forever); got %s", muteDuration), false, nil)
		}
	}

	if err := uc.chats.MuteChat(ctx, userID, chatJID, req.Mute, muteDuration); err != nil {
		uc.logger.Error(ctx, "failed to mute chat", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to mute chat: %w", err)
	}

	msg := "Chat muted"
	if !req.Mute {
		msg = "Chat unmuted"
	}

	return &domain.MuteChatResult{
		Success: true,
		Message: msg,
	}, nil
}
