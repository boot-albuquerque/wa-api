package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// LogoutUseCase encapsula a validação de logout.
type LogoutUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewLogoutUseCase cria uma nova instância do usecase.
func NewLogoutUseCase(sg appport.SessionGuard, l appport.Logger) *LogoutUseCase {
	return &LogoutUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível e logado.
func (uc *LogoutUseCase) Execute(ctx context.Context, txtID string, req domain.LogoutRequest) (*domain.LogoutResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "logout validated", "txtID", txtID)
	return &domain.LogoutResult{}, nil
}
