package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// DeleteS3ConfigUseCase encapsula a validação de exclusão de configuração de S3.
type DeleteS3ConfigUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewDeleteS3ConfigUseCase cria uma nova instância do usecase.
func NewDeleteS3ConfigUseCase(cp port.ClientProvider, l port.Logger) *DeleteS3ConfigUseCase {
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
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.S3ConfigResult{
		Details: "S3 configuration deleted",
		Enabled: false,
	}

	uc.logger.Info("S3 configuration deleted", "txtID", txtID)
	return result, nil
}
