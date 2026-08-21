package session

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SetStatusMessageUseCase encapsula a validação de definição de status.
type SetStatusMessageUseCase struct {
	status appport.StatusMessageSetter
	logger appport.Logger
}

// NewSetStatusMessageUseCase cria uma nova instância do usecase.
func NewSetStatusMessageUseCase(st appport.StatusMessageSetter, l appport.Logger) *SetStatusMessageUseCase {
	return &SetStatusMessageUseCase{
		status: st,
		logger: l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SetStatusMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SetStatusMessageRequest) (*domain.SetStatusMessageResult, error) {
	if req.Body == "" {
		return nil, apperr.New("missing_body", apperr.CategoryValidation, "missing Body in payload", false, nil)
	}

	if err := uc.status.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	// F198: até 2026-08-21 o Execute PARAVA aqui, com um log a dizer
	// "validated" — que era honesto sobre o que o código fazia e mentiroso
	// sobre o que a rota promete. O estado do utilizador nunca mudava e o
	// cliente recebia 200.
	if err := uc.status.SetStatusMessage(ctx, txtID, req.Body); err != nil {
		uc.logger.Error(ctx, "set status message failed", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "status message set", "txtID", txtID)
	return &domain.SetStatusMessageResult{}, nil
}
