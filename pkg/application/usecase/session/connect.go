package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
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

// Execute returns a successful result so the handler can proceed
// with the actual connection flow (QR code generation + WebSocket).
func (uc *ConnectUseCase) Execute(ctx context.Context, txtID string, req domain.ConnectRequest) (*domain.ConnectResult, error) {
	uc.logger.Info(ctx, "connect requested", "txtID", txtID)
	return &domain.ConnectResult{}, nil
}
