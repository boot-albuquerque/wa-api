package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DeleteS3ConfigUseCase encapsula a validação de exclusão de configuração de S3.
type DeleteS3ConfigUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewDeleteS3ConfigUseCase cria uma nova instância do usecase.
func NewDeleteS3ConfigUseCase(cp appport.ClientProvider, l appport.Logger) *DeleteS3ConfigUseCase {
	return &DeleteS3ConfigUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *DeleteS3ConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.S3ConfigResult, error) {
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
		Details: "S3 configuration deleted",
		Enabled: false,
	}

	uc.logger.Info(ctx, "S3 configuration deleted", "txtID", txtID)
	return result, nil
}
