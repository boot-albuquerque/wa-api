package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/shared/domain"
)

// SendLocationUseCase encapsula a validação de envio de localização.
type SendLocationUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewSendLocationUseCase cria uma nova instância do usecase.
func NewSendLocationUseCase(cp port.ClientProvider, l port.Logger) *SendLocationUseCase {
	return &SendLocationUseCase{
		clientProvider: cp,
		logger:         l,
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

	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	msgID := req.ID
	if msgID == "" {
		msgID = client.GenerateMessageID()
	}

	result := &domain.SendLocationResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("location validated", "msgID", msgID)
	return result, nil
}
