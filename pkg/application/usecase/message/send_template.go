package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendTemplateUseCase encapsula a validação de envio de template.
type SendTemplateUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendTemplateUseCase cria uma nova instância do usecase.
func NewSendTemplateUseCase(mc appport.MessageComposer, l appport.Logger) *SendTemplateUseCase {
	return &SendTemplateUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendTemplateUseCase) Execute(ctx context.Context, txtID string, req domain.SendTemplateRequest) (*domain.SendTemplateResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Content == "" {
		return nil, fmt.Errorf("missing Content in payload")
	}
	if req.Footer == "" {
		return nil, fmt.Errorf("missing Footer in payload")
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

	result := &domain.SendTemplateResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "template validated", "msgID", msgID)
	return result, nil
}
