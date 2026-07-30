package usecase

import (
	"context"
	"fmt"

	"wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// SendListUseCase encapsula a validação de envio de lista.
type SendListUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewSendListUseCase cria uma nova instância do usecase.
func NewSendListUseCase(cp port.ClientProvider, l port.Logger) *SendListUseCase {
	return &SendListUseCase{
		clientProvider: cp,
		logger:         l,
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

	result := &domain.SendListResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("list validated", "msgID", msgID)
	return result, nil
}
