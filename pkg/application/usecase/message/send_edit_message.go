package message

import (
	"context"
	"fmt"

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
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in payload")
	}
	if req.ID == "" {
		return nil, fmt.Errorf("missing Id in payload")
	}

	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	result := &domain.SendEditMessageResult{
		MessageID: req.ID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "message edit validated", "msgID", req.ID)
	return result, nil
}
