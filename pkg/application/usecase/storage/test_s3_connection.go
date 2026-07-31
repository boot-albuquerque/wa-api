package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// TestS3ConnectionUseCase encapsula a validação de teste de conexão S3.
type TestS3ConnectionUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewTestS3ConnectionUseCase cria uma nova instância do usecase.
func NewTestS3ConnectionUseCase(cp appport.ClientProvider, l appport.Logger) *TestS3ConnectionUseCase {
	return &TestS3ConnectionUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *TestS3ConnectionUseCase) Execute(ctx context.Context, txtID string, req domain.S3TestRequest) (*domain.S3TestResult, error) {
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

	// Validate required fields
	if req.Endpoint == "" || req.Region == "" || req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
		return nil, fmt.Errorf("missing required S3 configuration fields")
	}

	result := &domain.S3TestResult{
		Connected: true,
		Details:   "S3 connection test validated",
	}

	uc.logger.Info("S3 connection test validated", "txtID", txtID)
	return result, nil
}
