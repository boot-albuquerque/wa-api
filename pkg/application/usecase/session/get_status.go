package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetStatusUseCase encapsula a validação de obtenção de status.
type GetStatusUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetStatusUseCase cria uma nova instância do usecase.
func NewGetStatusUseCase(cp appport.ClientProvider, l appport.Logger) *GetStatusUseCase {
	return &GetStatusUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *GetStatusUseCase) Execute(ctx context.Context, txtID string) (*domain.GetStatusResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error(ctx, "client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "get status validated", "txtID", txtID)
	return &domain.GetStatusResult{}, nil
}
