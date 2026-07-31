package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// SendVideoUseCase encapsula a validação de envio de vídeo.
type SendVideoUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendVideoUseCase cria uma nova instância do usecase.
func NewSendVideoUseCase(cp appport.ClientProvider, l appport.Logger) *SendVideoUseCase {
	return &SendVideoUseCase{
		clientProvider: cp,
		logger:         l,
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

	result := &domain.SendVideoResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("video validated", "msgID", msgID)
	return result, nil
}
