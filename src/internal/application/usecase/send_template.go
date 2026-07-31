package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// SendTemplateUseCase encapsula a validação de envio de template.
type SendTemplateUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendTemplateUseCase cria uma nova instância do usecase.
func NewSendTemplateUseCase(cp appport.ClientProvider, l appport.Logger) *SendTemplateUseCase {
	return &SendTemplateUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
func (uc *SendTemplateUseCase) Execute(ctx context.Context, txtID string, req domain.SendTemplateRequest) (*domain.SendTemplateResult, error) {
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Content == "" {
		return nil, fmt.Errorf("missing Content in payload")
	}
	if req.Footer == "" {
		return nil, fmt.Errorf("missing Footer in payload")
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

	result := &domain.SendTemplateResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("template validated", "msgID", msgID)
	return result, nil
}
