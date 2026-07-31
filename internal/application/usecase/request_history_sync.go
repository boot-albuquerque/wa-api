package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// RequestHistorySyncUseCase encapsula a validação de requisição de sincronização de histórico.
type RequestHistorySyncUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewRequestHistorySyncUseCase cria uma nova instância do usecase.
func NewRequestHistorySyncUseCase(cp port.ClientProvider, l port.Logger) *RequestHistorySyncUseCase {
	return &RequestHistorySyncUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida se o cliente está disponível.
func (uc *RequestHistorySyncUseCase) Execute(ctx context.Context, txtID string, req domain.RequestHistorySyncRequest) (*domain.RequestHistorySyncResult, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("request history sync validated", "txtID", txtID)
	return &domain.RequestHistorySyncResult{}, nil
}
