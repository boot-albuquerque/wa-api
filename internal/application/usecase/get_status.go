package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// GetStatusUseCase encapsula a validação de obtenção de status.
type GetStatusUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewGetStatusUseCase cria uma nova instância do usecase.
func NewGetStatusUseCase(cp port.ClientProvider, l port.Logger) *GetStatusUseCase {
	return &GetStatusUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetStatusUseCase) Execute(ctx context.Context, txtID string) (*domain.GetStatusResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("get status validated", "txtID", txtID)
	return &domain.GetStatusResult{}, nil
}
