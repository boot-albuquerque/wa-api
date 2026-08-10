package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendEditMessageUseCase encapsula a validação de edição de mensagem.
type SendEditMessageUseCase struct {
	sessions appport.SessionGuard
	logger   appport.Logger
}

// NewSendEditMessageUseCase cria uma nova instância do usecase.
func NewSendEditMessageUseCase(sg appport.SessionGuard, l appport.Logger) *SendEditMessageUseCase {
	return &SendEditMessageUseCase{
		sessions: sg,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendEditMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SendEditMessageRequest) (*domain.SendEditMessageResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Body == "" {
		return nil, apperr.New("missing_body", apperr.CategoryValidation, "missing Body in payload", false, nil)
	}
	if req.ID == "" {
		return nil, apperr.New("missing_id", apperr.CategoryValidation, "missing Id in payload", false, nil)
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendEditMessageResult{
		MessageID: req.ID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "message edit validated", "msgID", req.ID)
	return result, nil
}
