package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendLocationUseCase encapsula a validação de envio de localização.
type SendLocationUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendLocationUseCase cria uma nova instância do usecase.
func NewSendLocationUseCase(mc appport.MessageComposer, l appport.Logger) *SendLocationUseCase {
	return &SendLocationUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendLocationUseCase) Execute(ctx context.Context, txtID string, req domain.SendLocationRequest) (*domain.SendLocationResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Latitude == 0 {
		return nil, fmt.Errorf("missing Latitude in payload")
	}
	if req.Longitude == 0 {
		return nil, fmt.Errorf("missing Longitude in payload")
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

	result := &domain.SendLocationResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "location validated", "msgID", msgID)
	return result, nil
}
