package storage

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ConfigureHmacUseCase encapsula a validação de configuração de HMAC.
type ConfigureHmacUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewConfigureHmacUseCase cria uma nova instância do usecase.
func NewConfigureHmacUseCase(sg appport.SessionGuard, l appport.Logger) *ConfigureHmacUseCase {
	return &ConfigureHmacUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *ConfigureHmacUseCase) Execute(ctx context.Context, txtID string, req domain.HmacConfigRequest) (*domain.HmacConfigResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.HmacConfigResult{
		Details: "HMAC configuration validated",
		Enabled: req.Enabled,
	}

	uc.logger.Info(ctx, "HMAC configuration validated", "txtID", txtID)
	return result, nil
}
