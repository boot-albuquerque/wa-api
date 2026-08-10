package chat

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

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
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.Chat == "" {
		return nil, apperr.New("missing_chat", apperr.CategoryValidation, "missing Chat in Payload", false, nil)
	}

	if req.Sender == "" {
		return nil, apperr.New("missing_sender", apperr.CategoryValidation, "missing Sender in Payload", false, nil)
	}

	if req.ID == "" {
		return nil, apperr.New("missing_id", apperr.CategoryValidation, "missing ID in Payload", false, nil)
	}

	chatJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Chat)
	if err != nil {
		return nil, apperr.New("invalid_chat_jid", apperr.CategoryValidation, "invalid Chat JID format", false, nil)
	}

	senderJID, err := uc.jids.ResolveQualifiedJID(ctx, req.Sender)
	if err != nil {
		return nil, apperr.New("invalid_sender_jid", apperr.CategoryValidation, "invalid Sender JID format", false, nil)
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
