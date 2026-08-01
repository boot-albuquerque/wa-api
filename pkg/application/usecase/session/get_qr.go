package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetQRUseCase encapsula a validação de obtenção do QR code.
type GetQRUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewGetQRUseCase cria uma nova instância do usecase.
func NewGetQRUseCase(sg appport.SessionGuard, l appport.Logger) *GetQRUseCase {
	return &GetQRUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetQRUseCase) Execute(ctx context.Context, txtID string) (*domain.GetQRResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "get QR validated", "txtID", txtID)
	return &domain.GetQRResult{}, nil
}
