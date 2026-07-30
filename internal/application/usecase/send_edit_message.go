package usecase

import (
	"context"
	"fmt"

	"wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// SendEditMessageUseCase encapsula a validação de edição de mensagem.
type SendEditMessageUseCase struct {
	clientProvider port.ClientProvider
	logger         port.Logger
}

// NewSendEditMessageUseCase cria uma nova instância do usecase.
func NewSendEditMessageUseCase(cp port.ClientProvider, l port.Logger) *SendEditMessageUseCase {
	return &SendEditMessageUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendEditMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SendEditMessageRequest) (*domain.SendEditMessageResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in payload")
	}
	if req.ID == "" {
		return nil, fmt.Errorf("missing Id in payload")
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

	result := &domain.SendEditMessageResult{
		MessageID: req.ID,
		Status:    "validated",
	}

	uc.logger.Info("message edit validated", "msgID", req.ID)
	return result, nil
}
