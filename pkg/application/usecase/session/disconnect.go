package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DisconnectUseCase encapsula a validação de desconexão.
type DisconnectUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewDisconnectUseCase cria uma nova instância do usecase.
func NewDisconnectUseCase(sg appport.SessionGuard, l appport.Logger) *DisconnectUseCase {
	return &DisconnectUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está conectado.
func (uc *DisconnectUseCase) Execute(ctx context.Context, txtID string, req domain.DisconnectRequest) (*domain.DisconnectResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "disconnect validated", "txtID", txtID)
	return &domain.DisconnectResult{}, nil
}
