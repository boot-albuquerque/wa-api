package usecase

import (
	"context"
	"fmt"

	"disparazap/internal/application/port"
	"disparazap/internal/domain"
)

// SendPollUseCase encapsula a validação de envio de enquete.
type SendPollUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewSendPollUseCase cria uma nova instância do usecase.
func NewSendPollUseCase(cp port.ClientProvider, l port.Logger) *SendPollUseCase {
	return &SendPollUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendPollUseCase) Execute(ctx context.Context, txtID string, req domain.SendPollRequest) (*domain.SendPollResult, error) {
	if req.Group == "" {
		return nil, fmt.Errorf("missing Group in payload")
	}
	if req.Header == "" {
		return nil, fmt.Errorf("missing Header in payload")
	}
	if len(req.Options) < 2 {
		return nil, fmt.Errorf("at least 2 options are required")
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

	result := &domain.SendPollResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("poll validated", "msgID", msgID)
	return result, nil
}
