package session

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SetStatusMessageUseCase encapsula a validação de definição de status.
type SetStatusMessageUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewSetStatusMessageUseCase cria uma nova instância do usecase.
func NewSetStatusMessageUseCase(sg appport.SessionGuard, l appport.Logger) *SetStatusMessageUseCase {
	return &SetStatusMessageUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SetStatusMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SetStatusMessageRequest) (*domain.SetStatusMessageResult, error) {
	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in payload")
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	uc.logger.Info(ctx, "set status message validated", "txtID", txtID)
	return &domain.SetStatusMessageResult{}, nil
}
