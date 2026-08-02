package chat

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ArchiveChatUseCase archives or unarchives a chat
type ArchiveChatUseCase struct {
	chats  appport.ChatOperations
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewArchiveChatUseCase creates a new instance
func NewArchiveChatUseCase(co appport.ChatOperations, jr appport.JIDResolver, logger appport.Logger) *ArchiveChatUseCase {
	return &ArchiveChatUseCase{chats: co, jids: jr, logger: logger}
}

// Execute archives or unarchives a chat
func (uc *ArchiveChatUseCase) Execute(ctx context.Context, userID string, req domain.ArchiveChatRequest) (*domain.ArchiveChatResult, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.Jid == "" {
		return nil, fmt.Errorf("missing jid in Payload")
	}

	chatJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Jid)
	if err != nil {
		return nil, fmt.Errorf("invalid Chat JID format")
	}

	if err := uc.chats.ArchiveChat(ctx, userID, chatJID, req.Archive); err != nil {
		uc.logger.Error(ctx, "failed to archive chat", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to archive chat: %w", err)
	}

	statusText := "Chat archived"
	if !req.Archive {
		statusText = "Chat unarchived"
	}

	return &domain.ArchiveChatResult{
		Success: true,
		Message: statusText,
	}, nil
}
