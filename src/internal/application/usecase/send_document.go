package usecase

import (
	"context"
	"fmt"

	appport "wa-api/internal/application/contracts"
	"wa-api/internal/domain"
)

// SendDocumentUseCase encapsula a validação de envio de documento.
// A lógica de envio complexa (upload, etc) fica no wrapper handlers.go.
type SendDocumentUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendDocumentUseCase cria uma nova instância do usecase.
func NewSendDocumentUseCase(cp appport.ClientProvider, l appport.Logger) *SendDocumentUseCase {
	return &SendDocumentUseCase{
		clientProvider: cp,
		logger:         l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
// Retorna os dados necessários ao wrapper handlers.go para enviar o documento.
// Esta é uma validação MVP — a lógica complexa fica no wrapper por enquanto.
func (uc *SendDocumentUseCase) Execute(ctx context.Context, txtID string, req domain.SendDocumentRequest) (*domain.SendDocumentResult, error) {
	// 1. Validar campos obrigatórios
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Document == "" {
		return nil, fmt.Errorf("missing Document in payload")
	}
	if req.FileName == "" {
		return nil, fmt.Errorf("missing FileName in payload")
	}

	// 2. Obter cliente whatsmeow para verificar se existe sessão
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, txtID)
	if err != nil {
		uc.logger.Error("failed to get whatsmeow client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}
	if client == nil {
		uc.logger.Error("client is nil", "txtID", txtID)
		return nil, fmt.Errorf("no session")
	}

	// 3. Gerar message ID se não fornecido
	msgID := req.ID
	if msgID == "" {
		msgID = client.GenerateMessageID()
	}

	// 4. Retornar resultado com dados validados
	// O wrapper handlers.go irá processar upload, etc.
	result := &domain.SendDocumentResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("document validated", "msgID", msgID)
	return result, nil
}
