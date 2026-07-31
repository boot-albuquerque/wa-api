package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// SetProxyUseCase encapsula a validação de configuração de Proxy.
type SetProxyUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewSetProxyUseCase cria uma nova instância do usecase.
func NewSetProxyUseCase(cp port.ClientProvider, l port.Logger) *SetProxyUseCase {
	return &SetProxyUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SetProxyUseCase) Execute(ctx context.Context, txtID string, req domain.ProxyConfigRequest) (*domain.ProxyConfigResult, error) {
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

	// If enabled, validate URL is provided
	if req.Enabled && req.URL == "" {
		return nil, fmt.Errorf("proxy URL is required when proxy is enabled")
	}

	result := &domain.ProxyConfigResult{
		Details: "Proxy configuration validated",
		Set:     true,
	}

	uc.logger.Info("Proxy configuration validated", "txtID", txtID)
	return result, nil
}
