package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// DisconnectUseCase encapsula a validação de desconexão.
type DisconnectUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewDisconnectUseCase cria uma nova instância do usecase.
func NewDisconnectUseCase(cp port.ClientProvider, l port.Logger) *DisconnectUseCase {
	return &DisconnectUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está conectado.
func (uc *DisconnectUseCase) Execute(ctx context.Context, txtID string, req domain.DisconnectRequest) (*domain.DisconnectResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("disconnect validated", "txtID", txtID)
	return &domain.DisconnectResult{}, nil
}
