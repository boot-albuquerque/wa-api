package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/shared/domain"
)

// GetQRUseCase encapsula a validação de obtenção do QR code.
type GetQRUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewGetQRUseCase cria uma nova instância do usecase.
func NewGetQRUseCase(cp port.ClientProvider, l port.Logger) *GetQRUseCase {
	return &GetQRUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetQRUseCase) Execute(ctx context.Context, txtID string) (*domain.GetQRResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("get QR validated", "txtID", txtID)
	return &domain.GetQRResult{}, nil
}
