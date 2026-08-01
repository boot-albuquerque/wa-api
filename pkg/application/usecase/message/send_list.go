package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendListUseCase encapsula a validação de envio de lista.
type SendListUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendListUseCase cria uma nova instância do usecase.
func NewSendListUseCase(mc appport.MessageComposer, l appport.Logger) *SendListUseCase {
	return &SendListUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendListUseCase) Execute(ctx context.Context, txtID string, req domain.SendListRequest) (*domain.SendListResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Desc == "" {
		return nil, fmt.Errorf("missing Desc in payload")
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	msgID := req.ID
	if msgID == "" {
		generated, err := uc.messages.NewMessageID(ctx, txtID)
		if err != nil {
			uc.logger.Error(ctx, "failed to generate message ID", "txtID", txtID, "error", err)
			return nil, fmt.Errorf("no session")
		}
		msgID = generated
	}

	result := &domain.SendListResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "list validated", "msgID", msgID)
	return result, nil
}
