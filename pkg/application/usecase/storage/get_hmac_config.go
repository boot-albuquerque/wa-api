package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetHmacConfigUseCase encapsula a validação de leitura de configuração de HMAC.
type GetHmacConfigUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewGetHmacConfigUseCase cria uma nova instância do usecase.
func NewGetHmacConfigUseCase(sg appport.SessionGuard, l appport.Logger) *GetHmacConfigUseCase {
	return &GetHmacConfigUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetHmacConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.HmacConfigResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.HmacConfigResult{
		Details: "HMAC configuration retrieved",
	}

	uc.logger.Info(ctx, "HMAC configuration retrieved", "txtID", txtID)
	return result, nil
}
