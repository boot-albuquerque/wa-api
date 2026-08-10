package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendContactUseCase encapsula a validação de envio de contato.
type SendContactUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendContactUseCase cria uma nova instância do usecase.
func NewSendContactUseCase(mc appport.MessageComposer, l appport.Logger) *SendContactUseCase {
	return &SendContactUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendContactUseCase) Execute(ctx context.Context, txtID string, req domain.SendContactRequest) (*domain.SendContactResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Name == "" {
		return nil, apperr.New("missing_name", apperr.CategoryValidation, "missing Name in payload", false, nil)
	}
	if req.Vcard == "" {
		return nil, apperr.New("missing_vcard", apperr.CategoryValidation, "missing Vcard in payload", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	msgID := req.ID
	if msgID == "" {
		generated, err := uc.messages.NewMessageID(ctx, txtID)
		if err != nil {
			uc.logger.Error(ctx, "failed to generate message ID", "txtID", txtID, "error", err)
			return nil, err
		}
		msgID = generated
	}

	result := &domain.SendContactResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "contact validated", "msgID", msgID)
	return result, nil
}
