package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// GetHistoryUseCase encapsula a validação de leitura de configuração de histórico.
type GetHistoryUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewGetHistoryUseCase cria uma nova instância do usecase.
func NewGetHistoryUseCase(cp port.ClientProvider, l port.Logger) *GetHistoryUseCase {
	return &GetHistoryUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetHistoryUseCase) Execute(ctx context.Context, txtID string) (*domain.WebhookHistoryResult, error) {
	// Validate client exists
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.WebhookHistoryResult{
		Details: "History configuration retrieved",
	}

	uc.logger.Info("History configuration retrieved", "txtID", txtID)
	return result, nil
}
