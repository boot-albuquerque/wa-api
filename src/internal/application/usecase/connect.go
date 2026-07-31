package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/contracts"
	"wa-api/internal/shared/domain"
)

// ConnectUseCase encapsula a validação de conexão.
type ConnectUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewConnectUseCase cria uma nova instância do usecase.
func NewConnectUseCase(cp appport.ClientProvider, l appport.Logger) *ConnectUseCase {
	return &ConnectUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *ConnectUseCase) Execute(ctx context.Context, txtID string, req domain.ConnectRequest) (*domain.ConnectResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("connect validated", "txtID", txtID)
	return &domain.ConnectResult{}, nil
}
