package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SetHistoryUseCase encapsula a validação de configuração de histórico.
type SetHistoryUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewSetHistoryUseCase cria uma nova instância do usecase.
func NewSetHistoryUseCase(sg appport.SessionGuard, l appport.Logger) *SetHistoryUseCase {
	return &SetHistoryUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SetHistoryUseCase) Execute(ctx context.Context, txtID string, req domain.WebhookHistoryRequest) (*domain.WebhookHistoryResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
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
