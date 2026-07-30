package usecase

import (
	"context"
	"fmt"

	"wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// SetHistoryUseCase encapsula a validação de configuração de histórico.
type SetHistoryUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewSetHistoryUseCase cria uma nova instância do usecase.
func NewSetHistoryUseCase(cp port.ClientProvider, l port.Logger) *SetHistoryUseCase {
	return &SetHistoryUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SetHistoryUseCase) Execute(ctx context.Context, txtID string, req domain.WebhookHistoryRequest) (*domain.WebhookHistoryResult, error) {
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

	// Validate history value
	if req.History < 0 {
		return nil, fmt.Errorf("history value cannot be negative")
	}

	result := &domain.WebhookHistoryResult{
		Details: "History configuration validated",
		History: req.History,
	}

	uc.logger.Info("History configuration validated", "txtID", txtID)
	return result, nil
}
