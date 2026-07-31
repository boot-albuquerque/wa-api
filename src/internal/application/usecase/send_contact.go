package usecase

import (
	"context"
	"fmt"

	appport "disparazap/internal/contracts"
	"disparazap/internal/shared/domain"
)

// SendContactUseCase encapsula a validação de envio de contato.
type SendContactUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendContactUseCase cria uma nova instância do usecase.
func NewSendContactUseCase(cp appport.ClientProvider, l appport.Logger) *SendContactUseCase {
	return &SendContactUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendContactUseCase) Execute(ctx context.Context, txtID string, req domain.SendContactRequest) (*domain.SendContactResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("missing Name in payload")
	}
	if req.Vcard == "" {
		return nil, fmt.Errorf("missing Vcard in payload")
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

	result := &domain.SendContactResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("contact validated", "msgID", msgID)
	return result, nil
}
