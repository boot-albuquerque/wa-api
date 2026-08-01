package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetHmacConfigUseCase encapsula a validação de leitura de configuração de HMAC.
type GetHmacConfigUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetHmacConfigUseCase cria uma nova instância do usecase.
func NewGetHmacConfigUseCase(cp appport.ClientProvider, l appport.Logger) *GetHmacConfigUseCase {
	return &GetHmacConfigUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetHmacConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.HmacConfigResult, error) {
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
		Details: "HMAC configuration retrieved",
	}

	uc.logger.Info(ctx, "HMAC configuration retrieved", "txtID", txtID)
	return result, nil
}
