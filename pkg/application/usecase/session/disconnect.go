package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DisconnectUseCase encapsula a validação de desconexão.
type DisconnectUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewDisconnectUseCase cria uma nova instância do usecase.
func NewDisconnectUseCase(cp appport.ClientProvider, l appport.Logger) *DisconnectUseCase {
	return &DisconnectUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está conectado.
func (uc *DisconnectUseCase) Execute(ctx context.Context, txtID string, req domain.DisconnectRequest) (*domain.DisconnectResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error(ctx, "client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "disconnect validated", "txtID", txtID)
	return &domain.DisconnectResult{}, nil
}
