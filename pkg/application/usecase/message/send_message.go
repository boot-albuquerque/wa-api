package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendMessageUseCase envia uma mensagem de texto de verdade pelo wa-noise.
type SendMessageUseCase struct {
	messages appport.TextMessenger
	jids     appport.JIDResolver
	previews appport.LinkPreviewFetcher
	logger   appport.Logger
}

// NewSendMessageUseCase cria uma nova instância do usecase.
func NewSendMessageUseCase(tm appport.TextMessenger, jr appport.JIDResolver, lpf appport.LinkPreviewFetcher, l appport.Logger) *SendMessageUseCase {
	return &SendMessageUseCase{
		messages: tm,
		jids:     jr,
		previews: lpf,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário e envia o
// texto pela porta de verdade. Só devolve domain.StatusSent depois que o
// envio retorna sucesso — nunca antes.
func (uc *SendMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SendMessageRequest) (*domain.SendMessageResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Body == "" {
		return nil, apperr.New("missing_body", apperr.CategoryValidation, "missing Body in payload", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send text payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	var preview *domain.LinkPreviewData
	if req.LinkPreview {
		if data, found := uc.previews.FetchLinkPreview(ctx, req.Body); found {
			preview = &data
		}
	}

	sent, err := uc.messages.SendText(ctx, txtID, recipient, req.Body, preview, req.ReplyTo, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send text message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendMessageResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "message sent", "msgID", result.MessageID)
	return result, nil
}
