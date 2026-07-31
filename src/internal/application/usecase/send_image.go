package usecase

import (
	"context"
	"fmt"

	appport "disparazap/internal/contracts"
	"disparazap/internal/shared/domain"
)

// SendImageUseCase encapsula a validação de envio de imagem.
// A lógica de envio complexa (upload, thumbnail, context info, etc) fica no wrapper handlers.go.
type SendImageUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSendImageUseCase cria uma nova instância do usecase.
func NewSendImageUseCase(cp appport.ClientProvider, l appport.Logger) *SendImageUseCase {
	return &SendImageUseCase{
		clientProvider: cp,
		logger:         l,
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
	// O wrapper handlers.go irá processar upload, thumbnail, context info, etc.
	result := &domain.SendImageResult{
		MessageID: msgID,
		Status:    "validated",
	}

	uc.logger.Info("image validated", "msgID", msgID)
	return result, nil
}
