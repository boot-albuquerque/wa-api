package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DeleteHmacConfigUseCase encapsula a validação de exclusão de configuração de HMAC.
type DeleteHmacConfigUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewDeleteHmacConfigUseCase cria uma nova instância do usecase.
func NewDeleteHmacConfigUseCase(sg appport.SessionGuard, l appport.Logger) *DeleteHmacConfigUseCase {
	return &DeleteHmacConfigUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *DeleteHmacConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.HmacConfigResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.HmacConfigResult{
		Details: "HMAC configuration deleted",
		Enabled: false,
	}

	uc.logger.Info(ctx, "HMAC configuration deleted", "txtID", txtID)
	return result, nil
}
