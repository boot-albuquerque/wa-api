package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetStatusUseCase encapsula a validação de obtenção de status.
type GetStatusUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewGetStatusUseCase cria uma nova instância do usecase.
func NewGetStatusUseCase(sg appport.SessionGuard, l appport.Logger) *GetStatusUseCase {
	return &GetStatusUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetStatusUseCase) Execute(ctx context.Context, txtID string) (*domain.GetStatusResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "get status validated", "txtID", txtID)
	return &domain.GetStatusResult{}, nil
}
