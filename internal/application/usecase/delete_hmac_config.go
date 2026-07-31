package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// DeleteHmacConfigUseCase encapsula a validação de exclusão de configuração de HMAC.
type DeleteHmacConfigUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewDeleteHmacConfigUseCase cria uma nova instância do usecase.
func NewDeleteHmacConfigUseCase(cp port.ClientProvider, l port.Logger) *DeleteHmacConfigUseCase {
	return &DeleteHmacConfigUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *DeleteHmacConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.HmacConfigResult, error) {
	// Validate client exists
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.HmacConfigResult{
		Details: "HMAC configuration deleted",
		Enabled: false,
	}

	uc.logger.Info("HMAC configuration deleted", "txtID", txtID)
	return result, nil
}
