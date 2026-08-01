package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DeleteMessageUseCase encapsula a validação de exclusão de mensagem.
type DeleteMessageUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewDeleteMessageUseCase cria uma nova instância do usecase.
func NewDeleteMessageUseCase(sg appport.SessionGuard, l appport.Logger) *DeleteMessageUseCase {
	return &DeleteMessageUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *DeleteMessageUseCase) Execute(ctx context.Context, txtID string, req domain.DeleteMessageRequest) (*domain.DeleteMessageResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.ID == "" {
		return nil, fmt.Errorf("missing Id in payload")
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.DeleteMessageResult{
		MessageID: req.ID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "message delete validated", "msgID", req.ID)
	return result, nil
}
