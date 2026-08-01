package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendVideoUseCase encapsula a validação de envio de vídeo.
type SendVideoUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendVideoUseCase cria uma nova instância do usecase.
func NewSendVideoUseCase(mc appport.MessageComposer, l appport.Logger) *SendVideoUseCase {
	return &SendVideoUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendVideoUseCase) Execute(ctx context.Context, txtID string, req domain.SendVideoRequest) (*domain.SendVideoResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Video == "" {
		return nil, fmt.Errorf("missing Video in payload")
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	msgID := req.ID
	if msgID == "" {
		generated, err := uc.messages.NewMessageID(ctx, txtID)
		if err != nil {
			uc.logger.Error(ctx, "failed to generate message ID", "txtID", txtID, "error", err)
			return nil, fmt.Errorf("no session")
		}
		msgID = generated
	}

	result := &domain.SendVideoResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "video validated", "msgID", msgID)
	return result, nil
}
