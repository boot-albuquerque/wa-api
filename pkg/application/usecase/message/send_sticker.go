package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendStickerUseCase encapsula a validação de envio de sticker.
type SendStickerUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendStickerUseCase cria uma nova instância do usecase.
func NewSendStickerUseCase(mc appport.MessageComposer, l appport.Logger) *SendStickerUseCase {
	return &SendStickerUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendStickerUseCase) Execute(ctx context.Context, txtID string, req domain.SendStickerRequest) (*domain.SendStickerResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Sticker == "" {
		return nil, fmt.Errorf("missing Sticker in payload")
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

	result := &domain.SendStickerResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "sticker validated", "msgID", msgID)
	return result, nil
}
