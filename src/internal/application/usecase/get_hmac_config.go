package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
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
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.HmacConfigResult{
		Details: "HMAC configuration retrieved",
	}

	uc.logger.Info("HMAC configuration retrieved", "txtID", txtID)
	return result, nil
}
