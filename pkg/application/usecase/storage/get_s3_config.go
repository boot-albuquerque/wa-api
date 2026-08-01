package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetS3ConfigUseCase encapsula a validação de leitura de configuração de S3.
type GetS3ConfigUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewGetS3ConfigUseCase cria uma nova instância do usecase.
func NewGetS3ConfigUseCase(sg appport.SessionGuard, l appport.Logger) *GetS3ConfigUseCase {
	return &GetS3ConfigUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetS3ConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.S3ConfigResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.S3ConfigResult{
		Details: "S3 configuration retrieved",
	}

	uc.logger.Info(ctx, "S3 configuration retrieved", "txtID", txtID)
	return result, nil
}
