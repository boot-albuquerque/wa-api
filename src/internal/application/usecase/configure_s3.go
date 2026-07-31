package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/contracts"
	"wa-api/internal/shared/domain"
)

// ConfigureS3UseCase encapsula a validação de configuração de S3.
type ConfigureS3UseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewConfigureS3UseCase cria uma nova instância do usecase.
func NewConfigureS3UseCase(cp appport.ClientProvider, l appport.Logger) *ConfigureS3UseCase {
	return &ConfigureS3UseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *ConfigureS3UseCase) Execute(ctx context.Context, txtID string, req domain.S3ConfigRequest) (*domain.S3ConfigResult, error) {
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

	// Validate media_delivery
	if req.MediaDelivery != "" && req.MediaDelivery != "base64" && req.MediaDelivery != "s3" && req.MediaDelivery != "both" {
		return nil, fmt.Errorf("media_delivery must be 'base64', 's3', or 'both'")
	}

	if req.MediaDelivery == "" {
		req.MediaDelivery = "base64"
	}

	result := &domain.S3ConfigResult{
		Details: "S3 configuration validated",
		Enabled: req.Enabled,
	}

	uc.logger.Info("S3 configuration validated", "txtID", txtID)
	return result, nil
}
