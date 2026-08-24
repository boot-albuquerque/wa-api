package chat

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// PinChatUseCase pins or unpins a chat in the conversation list.
type PinChatUseCase struct {
	chats  appport.ChatPinner
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewPinChatUseCase creates a new instance.
func NewPinChatUseCase(cp appport.ChatPinner, jr appport.JIDResolver, logger appport.Logger) *PinChatUseCase {
	return &PinChatUseCase{chats: cp, jids: jr, logger: logger}
}

// Execute pins or unpins a chat.
func (uc *PinChatUseCase) Execute(ctx context.Context, userID string, req domain.PinChatRequest) (*domain.PinChatResult, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.Jid == "" {
		return nil, apperr.New("missing_jid", apperr.CategoryValidation, "missing jid in Payload", false, nil)
	}

	chatJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Jid)
	if err != nil {
		return nil, apperr.New("invalid_chat_jid", apperr.CategoryValidation, "invalid Chat JID format", false, nil)
	}

	if err := uc.chats.PinChat(ctx, userID, chatJID, req.Pin); err != nil {
		uc.logger.Error(ctx, "failed to pin chat", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to pin chat: %w", err)
	}

	statusText := "Chat pinned"
	if !req.Pin {
		statusText = "Chat unpinned"
	}

	return &domain.PinChatResult{
		Success: true,
		Message: statusText,
	}, nil
}
