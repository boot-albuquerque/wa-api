package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetS3ConfigUseCase encapsula a validação de leitura de configuração de S3.
type GetS3ConfigUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetS3ConfigUseCase cria uma nova instância do usecase.
func NewGetS3ConfigUseCase(cp appport.ClientProvider, l appport.Logger) *GetS3ConfigUseCase {
	return &GetS3ConfigUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetS3ConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.S3ConfigResult, error) {
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

	result := &domain.S3ConfigResult{
		Details: "S3 configuration retrieved",
	}

	uc.logger.Info(ctx, "S3 configuration retrieved", "txtID", txtID)
	return result, nil
}
