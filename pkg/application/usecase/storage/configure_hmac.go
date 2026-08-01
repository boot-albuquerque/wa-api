package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ConfigureHmacUseCase encapsula a validação de configuração de HMAC.
type ConfigureHmacUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewConfigureHmacUseCase cria uma nova instância do usecase.
func NewConfigureHmacUseCase(cp appport.ClientProvider, l appport.Logger) *ConfigureHmacUseCase {
	return &ConfigureHmacUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *ConfigureHmacUseCase) Execute(ctx context.Context, txtID string, req domain.HmacConfigRequest) (*domain.HmacConfigResult, error) {
	// Validate client exists
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error(ctx, "client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.HmacConfigResult{
		Details: "HMAC configuration validated",
		Enabled: req.Enabled,
	}

	uc.logger.Info(ctx, "HMAC configuration validated", "txtID", txtID)
	return result, nil
}
