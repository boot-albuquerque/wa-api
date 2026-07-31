package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/shared/domain"
)

// DeleteMessageUseCase encapsula a validação de exclusão de mensagem.
type DeleteMessageUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewDeleteMessageUseCase cria uma nova instância do usecase.
func NewDeleteMessageUseCase(cp port.ClientProvider, l port.Logger) *DeleteMessageUseCase {
	return &DeleteMessageUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DeleteMessageUseCase) Execute(ctx context.Context, txtID string, req domain.DeleteMessageRequest) (*domain.DeleteMessageResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.ID == "" {
		return nil, fmt.Errorf("missing Id in payload")
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

	result := &domain.DeleteMessageResult{
		MessageID: req.ID,
		Status:    "validated",
	}

	uc.logger.Info("message delete validated", "msgID", req.ID)
	return result, nil
}
