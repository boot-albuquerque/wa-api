package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// SendAudioUseCase encapsula a validação de envio de áudio.
// A lógica de envio complexa (upload, etc) fica no wrapper handlers.go.
type SendAudioUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewSendAudioUseCase cria uma nova instância do usecase.
func NewSendAudioUseCase(cp port.ClientProvider, l port.Logger) *SendAudioUseCase {
	return &SendAudioUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendAudioUseCase) Execute(ctx context.Context, txtID string, req domain.SendAudioRequest) (*domain.SendAudioResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Audio == "" {
		return nil, fmt.Errorf("missing Audio in payload")
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

	result := &domain.SendAudioResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("audio validated", "msgID", msgID)
	return result, nil
}
