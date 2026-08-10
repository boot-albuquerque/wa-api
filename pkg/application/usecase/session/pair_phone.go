package session

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// PairPhoneUseCase encapsula a validação de pareamento por telefone.
type PairPhoneUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewPairPhoneUseCase cria uma nova instância do usecase.
func NewPairPhoneUseCase(sg appport.SessionGuard, l appport.Logger) *PairPhoneUseCase {
	return &PairPhoneUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *PairPhoneUseCase) Execute(ctx context.Context, txtID string, req domain.PairPhoneRequest) (*domain.PairPhoneResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "pair phone validated", "txtID", txtID)
	return &domain.PairPhoneResult{}, nil
}
