package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendAudioUseCase encapsula a validação de envio de áudio.
// A lógica de envio complexa (upload, etc) fica no wrapper handlers.go.
type SendAudioUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendAudioUseCase cria uma nova instância do usecase.
func NewSendAudioUseCase(mc appport.MessageComposer, l appport.Logger) *SendAudioUseCase {
	return &SendAudioUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendAudioUseCase) Execute(ctx context.Context, txtID string, req domain.SendAudioRequest) (*domain.SendAudioResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Audio == "" {
		return nil, apperr.New("missing_audio", apperr.CategoryValidation, "missing Audio in payload", false, nil)
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

	result := &domain.SendAudioResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "audio validated", "msgID", msgID)
	return result, nil
}
