package chat

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// muteForever is the duration BuildMute reads as "no expiry". Named because
// the number 0 says nothing on its own, and this file has to say it three
// times.
const muteForever time.Duration = 0

// Allowed mute durations. WhatsApp offers exactly three discrete options:
// 8 hours, 1 week, and forever (represented as zero duration to BuildMute).
var allowedMuteDurations = map[time.Duration]bool{
	8 * time.Hour:      true,
	7 * 24 * time.Hour: true,
	muteForever:        true,
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
		// Section 4.5 of CONTRATO-ARQUITETURAL: where the zero value carries a
		// meaning of its own, the field has to be a pointer AND the code has to
		// branch on nil BEFORE dereferencing — not after, with zero as the
		// default.
		//
		// The shape this replaces declared `var muteDuration time.Duration` and
		// only assigned it when the pointer was non-nil, so ABSENT fell through
		// into the same value as an explicit 0. Both mean "forever" today, and
		// the observable behaviour is unchanged — but they are DIFFERENT
		// inputs, and collapsing them is precisely how a misspelled field name
		// became a silent choice in F268: `duration` instead of `mute_duration`
		// left the pointer nil, nil became 0, and 0 muted the chat forever
		// under a 200.
		//
		// Written as two branches so that the day these need to diverge — a
		// distinct error for an explicit 0, say — the distinction is already
		// here to be used rather than having to be recovered.
		switch {
		case req.MuteDuration == nil:
			// Field absent from the payload: the caller expressed no
			// preference, and the contract says that means forever.
			muteDuration = muteForever
		default:
			// Field present: the caller chose, including when the choice is 0.
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
