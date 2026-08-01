package misc

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

// ArchiveChatUseCase archives or unarchives a chat
type ArchiveChatUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewArchiveChatUseCase creates a new instance
func NewArchiveChatUseCase(cp appport.ClientProvider, logger zerolog.Logger) *ArchiveChatUseCase {
	return &ArchiveChatUseCase{clientProvider: cp, logger: logger}
}

// Execute archives or unarchives a chat
func (uc *ArchiveChatUseCase) Execute(ctx context.Context, userID string, req domain.ArchiveChatRequest) (*domain.ArchiveChatResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	if req.Jid == "" {
		return nil, fmt.Errorf("missing jid in Payload")
	}

	chatJID, err := types.ParseJID(req.Jid)
	if err != nil {
		return nil, fmt.Errorf("invalid Chat JID format")
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	err = client.SendAppState(ctxWithTimeout, appstate.BuildArchive(chatJID, req.Archive, time.Time{}, nil))
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to archive chat")
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
