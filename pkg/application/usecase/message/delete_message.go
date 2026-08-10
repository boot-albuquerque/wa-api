package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

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
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.ID == "" {
		return nil, apperr.New("missing_id", apperr.CategoryValidation, "missing Id in payload", false, nil)
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.DeleteMessageResult{
		MessageID: req.ID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "message delete validated", "msgID", req.ID)
	return result, nil
}
