package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SetHistoryUseCase encapsula a validação de configuração de histórico.
type SetHistoryUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSetHistoryUseCase cria uma nova instância do usecase.
func NewSetHistoryUseCase(cp appport.ClientProvider, l appport.Logger) *SetHistoryUseCase {
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
		uc.logger.Error(ctx, "failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error(ctx, "client is nil", "txtID", txtID)
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

	uc.logger.Info(ctx, "History configuration validated", "txtID", txtID)
	return result, nil
}
