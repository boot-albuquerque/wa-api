package message

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendImageUseCase encapsula a validação de envio de imagem.
// A lógica de envio complexa (upload, thumbnail, context info, etc) fica no wrapper handlers.go.
type SendImageUseCase struct {
	messages appport.MessageComposer
	logger   appport.Logger
}

// NewSendImageUseCase cria uma nova instância do usecase.
func NewSendImageUseCase(mc appport.MessageComposer, l appport.Logger) *SendImageUseCase {
	return &SendImageUseCase{
		messages: mc,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios e verifica se o cliente está disponível.
// Retorna os dados necessários ao wrapper handlers.go para enviar a imagem.
// Esta é uma validação MVP — a lógica complexa fica no wrapper por enquanto.
func (uc *SendImageUseCase) Execute(ctx context.Context, txtID string, req domain.SendImageRequest) (*domain.SendImageResult, error) {
	// 1. Validar campos obrigatórios
	if req.Phone == "" {
		return nil, fmt.Errorf("missing Phone in payload")
	}
	if req.Image == "" {
		return nil, fmt.Errorf("missing Image in payload")
	}

	// 2. Obter cliente whatsmeow para verificar se existe sessão
	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "txtID", txtID, "error", err)
		return nil, fmt.Errorf("no session")
	}

	// 3. Gerar message ID se não fornecido
	msgID := req.ID
	if msgID == "" {
		generated, err := uc.messages.NewMessageID(ctx, txtID)
		if err != nil {
			uc.logger.Error(ctx, "failed to generate message ID", "txtID", txtID, "error", err)
			return nil, fmt.Errorf("no session")
		}
		msgID = generated
	}

	// 4. Retornar resultado com dados validados
	// O wrapper handlers.go irá processar upload, thumbnail, context info, etc.
	result := &domain.SendImageResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info(ctx, "image validated", "msgID", msgID)
	return result, nil
}
