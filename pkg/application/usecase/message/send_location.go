package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendLocationUseCase encapsula a validação de envio de localização.
type SendLocationUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendLocationUseCase cria uma nova instância do usecase.
func NewSendLocationUseCase(mc appport.MessageComposer, l appport.Logger) *SendLocationUseCase {
	return &SendLocationUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendLocationUseCase) Execute(ctx context.Context, txtID string, req domain.SendLocationRequest) (*domain.SendLocationResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Latitude == 0 {
		return nil, apperr.New("missing_latitude", apperr.CategoryValidation, "missing Latitude in payload", false, nil)
	}
	if req.Longitude == 0 {
		return nil, apperr.New("missing_longitude", apperr.CategoryValidation, "missing Longitude in payload", false, nil)
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

	result := &domain.SendLocationResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "location validated", "msgID", msgID)
	return result, nil
}
