package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// RequestHistorySyncUseCase encapsula a validação de requisição de sincronização de histórico.
type RequestHistorySyncUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewRequestHistorySyncUseCase cria uma nova instância do usecase.
func NewRequestHistorySyncUseCase(sg appport.SessionGuard, l appport.Logger) *RequestHistorySyncUseCase {
	return &RequestHistorySyncUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *RequestHistorySyncUseCase) Execute(ctx context.Context, txtID string, req domain.RequestHistorySyncRequest) (*domain.RequestHistorySyncResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "request history sync validated", "txtID", txtID)
	return &domain.RequestHistorySyncResult{}, nil
}
