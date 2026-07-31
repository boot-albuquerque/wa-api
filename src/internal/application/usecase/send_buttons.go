package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/contracts"
	"wa-api/internal/shared/domain"
)

// SendButtonsUseCase encapsula a validação de envio de botões.
type SendButtonsUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendButtonsUseCase cria uma nova instância do usecase.
func NewSendButtonsUseCase(cp appport.ClientProvider, l appport.Logger) *SendButtonsUseCase {
	return &SendButtonsUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendButtonsUseCase) Execute(ctx context.Context, txtID string, req domain.SendButtonsRequest) (*domain.SendButtonsResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in payload")
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

	result := &domain.SendButtonsResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("buttons validated", "msgID", msgID)
	return result, nil
}
