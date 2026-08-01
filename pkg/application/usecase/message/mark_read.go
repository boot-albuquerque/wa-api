package message

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/whatsmeow"

	"go.mau.fi/whatsmeow/types"
)

// MarkReadUseCase marks messages as read
type MarkReadUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewMarkReadUseCase creates a new instance
func NewMarkReadUseCase(cp appport.ClientProvider, logger appport.Logger) *MarkReadUseCase {
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
		uc.logger.Error(ctx, "Failed to mark messages as read", "error", err, "user_id", userID)
		return fmt.Errorf("failure marking messages as read")
	}

	uc.logger.Info(ctx, "Messages marked as read", "user_id", userID, "count", len(req.Id))
	return nil
}
