package misc

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// RequestUnavailableMessageUseCase requests an unavailable message
type RequestUnavailableMessageUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewRequestUnavailableMessageUseCase creates a new instance
func NewRequestUnavailableMessageUseCase(cp appport.ClientProvider, logger zerolog.Logger) *RequestUnavailableMessageUseCase {
	return &RequestUnavailableMessageUseCase{clientProvider: cp, logger: logger}
}

// Execute requests an unavailable message
func (uc *RequestUnavailableMessageUseCase) Execute(ctx context.Context, userID string, req domain.RequestUnavailableMessageRequest) (*domain.RequestUnavailableMessageResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
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

	chatJID, err := types.ParseJID(req.Chat)
	if err != nil {
		return nil, fmt.Errorf("invalid Chat JID format")
	}

	senderJID, err := types.ParseJID(req.Sender)
	if err != nil {
		return nil, fmt.Errorf("invalid Sender JID format")
	}

	unavailableMessage := client.BuildUnavailableMessageRequest(chatJID, senderJID, req.ID)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := client.SendMessage(ctxWithTimeout, chatJID, unavailableMessage, whatsmeow.SendRequestExtra{Peer: true})
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to send unavailable message request")
		return nil, fmt.Errorf("failed to send unavailable message request: %w", err)
	}

	return &domain.RequestUnavailableMessageResult{
		Success:   true,
		Message:   "Unavailable message request sent successfully",
		RequestID: resp.ID,
		Chat:      req.Chat,
		Sender:    req.Sender,
		MessageID: req.ID,
		Timestamp: resp.Timestamp.Unix(),
	}, nil
}
