package storage

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DeleteS3ConfigUseCase encapsula a validação de exclusão de configuração de S3.
type DeleteS3ConfigUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewDeleteS3ConfigUseCase cria uma nova instância do usecase.
func NewDeleteS3ConfigUseCase(sg appport.SessionGuard, l appport.Logger) *DeleteS3ConfigUseCase {
	return &DeleteS3ConfigUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *DeleteS3ConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.S3ConfigResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.S3ConfigResult{
		Details: "S3 configuration deleted",
		Enabled: false,
	}

	uc.logger.Info(ctx, "S3 configuration deleted", "txtID", txtID)
	return result, nil
}
