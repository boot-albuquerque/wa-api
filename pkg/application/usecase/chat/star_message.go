package chat

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// StarMessageUseCase stars or unstars a message.
type StarMessageUseCase struct {
	stars  appport.MessageStarrer
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewStarMessageUseCase creates a new instance.
func NewStarMessageUseCase(ms appport.MessageStarrer, jr appport.JIDResolver, logger appport.Logger) *StarMessageUseCase {
	return &StarMessageUseCase{stars: ms, jids: jr, logger: logger}
}

// Execute stars or unstars a message.
func (uc *StarMessageUseCase) Execute(ctx context.Context, userID string, req domain.StarMessageRequest) (*domain.StarMessageResult, error) {
	if err := uc.stars.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.Chat == "" {
		return nil, apperr.New("missing_chat", apperr.CategoryValidation, "missing chat in payload", false, nil)
	}
	if req.MessageID == "" {
		return nil, apperr.New("missing_message_id", apperr.CategoryValidation, "missing message_id in payload", false, nil)
	}
	if req.Sender == "" {
		return nil, apperr.New("missing_sender", apperr.CategoryValidation, "missing sender in payload", false, nil)
	}

	chatJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Chat)
	if err != nil {
		return nil, apperr.New("invalid_chat_jid", apperr.CategoryValidation, "invalid chat JID format", false, nil)
	}

	senderJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Sender)
	if err != nil {
		return nil, apperr.New("invalid_sender_jid", apperr.CategoryValidation, "invalid sender JID format", false, nil)
	}

	if err := uc.stars.StarMessage(ctx, userID, chatJID, senderJID, req.MessageID, req.FromMe, req.Star); err != nil {
		uc.logger.Error(ctx, "failed to star message", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to star message: %w", err)
	}

	statusText := "Message starred"
	if !req.Star {
		statusText = "Message unstarred"
	}

	return &domain.StarMessageResult{
		Success: true,
		Message: statusText,
	}, nil
}
