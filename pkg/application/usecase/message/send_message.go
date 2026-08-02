package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendMessageUseCase encapsula a validação de mensagem de texto.
// A lógica de envio complexa (link preview, context info, etc) fica no wrapper handlers.go.
type SendMessageUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendMessageUseCase cria uma nova instância do usecase.
func NewSendMessageUseCase(mc appport.MessageComposer, l appport.Logger) *SendMessageUseCase {
	return &SendMessageUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
// Retorna os dados necessários ao wrapper handlers.go para enviar a mensagem.
// Esta é uma validação MVP — a lógica complexa fica no wrapper por enquanto.
func (uc *SendMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SendMessageRequest) (*domain.SendMessageResult, error) {
	// 1. Validar campos obrigatórios
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Body == "" {
		return nil, fmt.Errorf("missing Body in payload")
	}

	// 2. Obter cliente whatsmeow para verificar se existe sessão
	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, err
	}

	// 3. Gerar message ID se não fornecido
	msgID := req.ID
	if msgID == "" {
		generated, err := uc.messages.NewMessageID(ctx, txtID)
		if err != nil {
			uc.logger.Error(ctx, "failed to generate message ID", "txtID", txtID, "error", err)
			return nil, err
		}
		msgID = generated
	}

	// 4. Retornar resultado com dados validados
	// O wrapper handlers.go irá processar link preview, context info, etc.
	result := &domain.SendMessageResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "message validated", "msgID", msgID)
	return result, nil
}
