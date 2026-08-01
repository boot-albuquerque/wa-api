package message

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// MarkReadUseCase marks messages as read
type MarkReadUseCase struct {
	chats  appport.ChatMessenger
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewMarkReadUseCase creates a new instance
func NewMarkReadUseCase(cm appport.ChatMessenger, jr appport.JIDResolver, logger appport.Logger) *MarkReadUseCase {
	return &MarkReadUseCase{chats: cm, jids: jr, logger: logger}
}

// Execute marks messages as read
func (uc *MarkReadUseCase) Execute(ctx context.Context, userID string, req domain.MarkReadRequest) error {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		return fmt.Errorf("no session")
	}

	var jidChat domain.JID

	if len(req.ChatPhone) > 0 {
		var err error
		jidChat, err = uc.jids.ResolveJID(ctx, req.ChatPhone)
		if err != nil {
			return fmt.Errorf("could not parse ChatPhone")
		}
	} else if req.Chat != "" {
		jidChat = "" // legacy field parsing would go here
	} else {
		return fmt.Errorf("missing ChatPhone in Payload")
	}

	var jidSender domain.JID

	if len(req.SenderPhone) > 0 {
		var err error
		jidSender, err = uc.jids.ResolveJID(ctx, req.SenderPhone)
		if err != nil {
			return fmt.Errorf("could not parse SenderPhone")
		}
	} else if req.Sender != "" {
		jidSender = "" // legacy field parsing would go here
	}

	if len(req.Id) < 1 {
		return fmt.Errorf("missing Id in Payload")
	}

	if err := uc.chats.MarkRead(ctx, userID, req.Id, time.Now(), jidChat, jidSender); err != nil {
		uc.logger.Error(ctx, "Failed to mark messages as read", "error", err, "user_id", userID)
		return fmt.Errorf("failure marking messages as read")
	}

	uc.logger.Info(ctx, "Messages marked as read", "user_id", userID, "count", len(req.Id))
	return nil
}
