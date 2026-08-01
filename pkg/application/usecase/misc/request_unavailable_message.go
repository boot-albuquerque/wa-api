package misc

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// RequestUnavailableMessageUseCase requests an unavailable message
type RequestUnavailableMessageUseCase struct {
	chats  appport.ChatOperations
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewRequestUnavailableMessageUseCase creates a new instance
func NewRequestUnavailableMessageUseCase(co appport.ChatOperations, jr appport.JIDResolver, logger appport.Logger) *RequestUnavailableMessageUseCase {
	return &RequestUnavailableMessageUseCase{chats: co, jids: jr, logger: logger}
}

// Execute requests an unavailable message
func (uc *RequestUnavailableMessageUseCase) Execute(ctx context.Context, userID string, req domain.RequestUnavailableMessageRequest) (*domain.RequestUnavailableMessageResult, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		return nil, fmt.Errorf("no session")
	}

	if req.Chat == "" {
		return nil, fmt.Errorf("missing Chat in Payload")
	}

	if req.Sender == "" {
		return nil, fmt.Errorf("missing Sender in Payload")
	}

	if req.ID == "" {
		return nil, fmt.Errorf("missing ID in Payload")
	}

	chatJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Chat)
	if err != nil {
		return nil, fmt.Errorf("invalid Chat JID format")
	}

	senderJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid Sender JID format")
	}

	ack, err := uc.chats.RequestUnavailableMessage(ctx, userID, chatJID, senderJID, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send unavailable message request", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to send unavailable message request: %w", err)
	}

	return &domain.RequestUnavailableMessageResult{
		Success:   true,
		Message:   "Unavailable message request sent successfully",
		RequestID: ack.RequestID,
		Chat:      req.Chat,
		Sender:    req.Sender,
		MessageID: req.ID,
		Timestamp: ack.Timestamp.Unix(),
	}, nil
}
