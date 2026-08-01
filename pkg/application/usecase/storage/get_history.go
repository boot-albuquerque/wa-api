package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetHistoryUseCase encapsula a validação de leitura de configuração de histórico.
type GetHistoryUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewGetHistoryUseCase cria uma nova instância do usecase.
func NewGetHistoryUseCase(sg appport.SessionGuard, l appport.Logger) *GetHistoryUseCase {
	return &GetHistoryUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetHistoryUseCase) Execute(ctx context.Context, txtID string) (*domain.WebhookHistoryResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.WebhookHistoryResult{
		Details: "History configuration retrieved",
	}

	uc.logger.Info(ctx, "History configuration retrieved", "txtID", txtID)
	return result, nil
}
