package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendContactUseCase envia um contato de verdade: monta o ContactMessage a
// partir dos campos escalares do request (sem upload, sem fetch, sem
// conversão) e o envia pelo wa-noise. Só devolve domain.StatusSent depois
// que o envio retorna sucesso — nunca antes (mesma disciplina de
// SendMessageUseCase, CAP-01).
type SendContactUseCase struct {
	messages appport.SimpleMessenger
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewSendContactUseCase cria uma nova instância do usecase.
func NewSendContactUseCase(sm appport.SimpleMessenger, jr appport.JIDResolver, l appport.Logger) *SendContactUseCase {
	return &SendContactUseCase{
		messages: sm,
		jids:     jr,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário e envia o
// contato pela porta de verdade. req.Vcard é repassado como STRING crua,
// sem parse nem validação de formato — mesma disciplina do histórico.
func (uc *SendContactUseCase) Execute(ctx context.Context, txtID string, req domain.SendContactRequest) (*domain.SendContactResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Name == "" {
		return nil, apperr.New("missing_name", apperr.CategoryValidation, "missing Name in payload", false, nil)
	}
	if req.Vcard == "" {
		return nil, apperr.New("missing_vcard", apperr.CategoryValidation, "missing Vcard in payload", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send contact payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	payload := domain.ContactPayload{Name: req.Name, Vcard: req.Vcard}

	sent, err := uc.messages.SendContact(ctx, txtID, recipient, payload, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send contact message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendContactResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "contact sent", "msgID", result.MessageID)
	return result, nil
}
