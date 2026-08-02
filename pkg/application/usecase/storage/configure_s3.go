package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/egress"
)

// ConfigureS3UseCase encapsula a validação de configuração de S3.
type ConfigureS3UseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewConfigureS3UseCase cria uma nova instância do usecase.
func NewConfigureS3UseCase(sg appport.SessionGuard, l appport.Logger) *ConfigureS3UseCase {
	return &ConfigureS3UseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *ConfigureS3UseCase) Execute(ctx context.Context, txtID string, req domain.S3ConfigRequest) (*domain.S3ConfigResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	// Validate media_delivery
	if req.MediaDelivery != "" && req.MediaDelivery != "base64" && req.MediaDelivery != "s3" && req.MediaDelivery != "both" {
		return nil, fmt.Errorf("media_delivery must be 'base64', 's3', or 'both'")
	}

	// Endpoint is optional (empty means the default AWS S3 endpoint); when
	// set, it had no validation at all before this (sec/F24).
	if req.Endpoint != "" {
		if err := egress.ValidateOutboundURL(ctx, req.Endpoint); err != nil {
			return nil, fmt.Errorf("invalid S3 endpoint: %w", err)
		}
	}

	if req.MediaDelivery == "" {
		req.MediaDelivery = "base64"
	}

	result := &domain.S3ConfigResult{
		Details: "S3 configuration validated",
		Enabled: req.Enabled,
	}

	uc.logger.Info(ctx, "S3 configuration validated", "txtID", txtID)
	return result, nil
}
