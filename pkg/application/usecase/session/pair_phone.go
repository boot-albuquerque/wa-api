package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// PairPhoneUseCase encapsula a validação de pareamento por telefone.
type PairPhoneUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewPairPhoneUseCase cria uma nova instância do usecase.
func NewPairPhoneUseCase(cp appport.ClientProvider, l appport.Logger) *PairPhoneUseCase {
	return &PairPhoneUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *PairPhoneUseCase) Execute(ctx context.Context, txtID string, req domain.PairPhoneRequest) (*domain.PairPhoneResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
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

	uc.logger.Info("pair phone validated", "txtID", txtID)
	return &domain.PairPhoneResult{}, nil
}
