package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SetStatusMessageUseCase encapsula a validação de definição de status.
type SetStatusMessageUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSetStatusMessageUseCase cria uma nova instância do usecase.
func NewSetStatusMessageUseCase(cp appport.ClientProvider, l appport.Logger) *SetStatusMessageUseCase {
	return &SetStatusMessageUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SetStatusMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SetStatusMessageRequest) (*domain.SetStatusMessageResult, error) {
	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in payload")
	}

	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info("set status message validated", "txtID", txtID)
	return &domain.SetStatusMessageResult{}, nil
}
