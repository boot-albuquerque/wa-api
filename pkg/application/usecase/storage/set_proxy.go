package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/egress"
)

// SetProxyUseCase encapsula a validação de configuração de Proxy.
type SetProxyUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewSetProxyUseCase cria uma nova instância do usecase.
func NewSetProxyUseCase(sg appport.SessionGuard, l appport.Logger) *SetProxyUseCase {
	return &SetProxyUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SetProxyUseCase) Execute(ctx context.Context, txtID string, req domain.ProxyConfigRequest) (*domain.ProxyConfigResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	// If enabled, validate URL is provided and does not point at a
	// reserved/loopback address (sec/F24 — proxy endpoint had no
	// validation at all before this).
	if req.Enabled {
		if req.URL == "" {
			return nil, fmt.Errorf("proxy URL is required when proxy is enabled")
		}
		if err := egress.ValidateOutboundURL(ctx, req.URL); err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
	}

	result := &domain.ProxyConfigResult{
		Details: "Proxy configuration validated",
		Set:     true,
	}

	uc.logger.Info(ctx, "Proxy configuration validated", "txtID", txtID)
	return result, nil
}
