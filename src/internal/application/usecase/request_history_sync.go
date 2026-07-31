package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// RequestHistorySyncUseCase encapsula a validação de requisição de sincronização de histórico.
type RequestHistorySyncUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewRequestHistorySyncUseCase cria uma nova instância do usecase.
func NewRequestHistorySyncUseCase(cp appport.ClientProvider, l appport.Logger) *RequestHistorySyncUseCase {
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
