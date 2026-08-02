package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendPollUseCase encapsula a validação de envio de enquete.
type SendPollUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendPollUseCase cria uma nova instância do usecase.
func NewSendPollUseCase(mc appport.MessageComposer, l appport.Logger) *SendPollUseCase {
	return &SendPollUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendPollUseCase) Execute(ctx context.Context, txtID string, req domain.SendPollRequest) (*domain.SendPollResult, error) {
	if req.Group == "" {
		return nil, fmt.Errorf("missing Group in payload")
	}
	if req.Header == "" {
		return nil, fmt.Errorf("missing Header in payload")
	}
	if len(req.Options) < 2 {
		return nil, fmt.Errorf("at least 2 options are required")
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	msgID := req.ID
	if msgID == "" {
		generated, err := uc.messages.NewMessageID(ctx, txtID)
		if err != nil {
			uc.logger.Error(ctx, "failed to generate message ID", "txtID", txtID, "error", err)
			return nil, err
		}
		msgID = generated
	}

	result := &domain.SendPollResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "poll validated", "msgID", msgID)
	return result, nil
}
