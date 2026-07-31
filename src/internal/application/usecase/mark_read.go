package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
	appport "wa-api/internal/contracts"
	"wa-api/internal/shared/domain"
	"wa-api/internal/infra/whatsmeow"
)

// MarkReadUseCase marks messages as read
type MarkReadUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewMarkReadUseCase creates a new instance
func NewMarkReadUseCase(cp appport.ClientProvider, logger zerolog.Logger) *MarkReadUseCase {
	return &MarkReadUseCase{clientProvider: cp, logger: logger}
}

// Execute marks messages as read
func (uc *MarkReadUseCase) Execute(ctx context.Context, userID string, req domain.MarkReadRequest) error {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return fmt.Errorf("no session")
	}

	var jidChat types.JID

	if len(req.ChatPhone) > 0 {
		var ok bool
		jidChat, ok = whatsmeow.ParseJID(req.ChatPhone)
		if !ok {
			return fmt.Errorf("could not parse ChatPhone")
		}
	} else if req.Chat != "" {
		jidChat = types.JID{} // legacy field parsing would go here
	} else {
		return fmt.Errorf("missing ChatPhone in Payload")
	}

	var jidSender types.JID

	if len(req.SenderPhone) > 0 {
		var ok bool
		jidSender, ok = whatsmeow.ParseJID(req.SenderPhone)
		if !ok {
			return fmt.Errorf("could not parse SenderPhone")
		}
	} else if req.Sender != "" {
		jidSender = types.JID{} // legacy field parsing would go here
	}

	if len(req.Id) < 1 {
		return fmt.Errorf("missing Id in Payload")
	}

	if err := client.MarkRead(ctx, req.Id, time.Now(), jidChat, jidSender); err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to mark messages as read")
		return fmt.Errorf("failure marking messages as read")
	}

	uc.logger.Info().Str("user_id", userID).Int("count", len(req.Id)).Msg("Messages marked as read")
	return nil
}
