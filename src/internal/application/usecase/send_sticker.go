package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// SendStickerUseCase encapsula a validação de envio de sticker.
type SendStickerUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendStickerUseCase cria uma nova instância do usecase.
func NewSendStickerUseCase(cp appport.ClientProvider, l appport.Logger) *SendStickerUseCase {
	return &SendStickerUseCase{
		clientProvider: cp,
		logger:         l,
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

	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	msgID := req.ID
	if msgID == "" {
		msgID = client.GenerateMessageID()
	}

	result := &domain.SendStickerResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("sticker validated", "msgID", msgID)
	return result, nil
}
