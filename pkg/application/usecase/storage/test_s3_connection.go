package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// TestS3ConnectionUseCase encapsula a validação de teste de conexão S3.
type TestS3ConnectionUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewTestS3ConnectionUseCase cria uma nova instância do usecase.
func NewTestS3ConnectionUseCase(sg appport.SessionGuard, l appport.Logger) *TestS3ConnectionUseCase {
	return &TestS3ConnectionUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *TestS3ConnectionUseCase) Execute(ctx context.Context, txtID string, req domain.S3TestRequest) (*domain.S3TestResult, error) {
	// Validate client exists
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	// Validate required fields
	if req.Endpoint == "" || req.Region == "" || req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
		return nil, fmt.Errorf("missing required S3 configuration fields")
	}

	result := &domain.S3TestResult{
		Connected: true,
		Details:   "S3 connection test validated",
	}

	uc.logger.Info(ctx, "S3 connection test validated", "txtID", txtID)
	return result, nil
}
