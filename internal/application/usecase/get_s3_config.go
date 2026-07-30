package usecase

import (
	"context"
	"fmt"

	"wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// GetS3ConfigUseCase encapsula a validação de leitura de configuração de S3.
type GetS3ConfigUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewGetS3ConfigUseCase cria uma nova instância do usecase.
func NewGetS3ConfigUseCase(cp port.ClientProvider, l port.Logger) *GetS3ConfigUseCase {
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
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.S3ConfigResult{
		Details: "S3 configuration retrieved",
	}

	uc.logger.Info("S3 configuration retrieved", "txtID", txtID)
	return result, nil
}
