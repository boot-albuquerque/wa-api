package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendTemplateUseCase envia um template hidratado de verdade: monta o payload
// a partir dos campos escalares do request (sem upload, sem fetch, sem
// conversão) e o envia pelo wa-noise. Só devolve domain.StatusSent depois que
// o envio retorna sucesso — nunca antes (mesma disciplina de
// SendMessageUseCase, CAP-01, e de SendPollUseCase, CAP-14).
//
// O que este bloco tem de diferente dos anteriores está registrado na
// HOUSEKEEP F139: não era só fiação morta. domain.SendTemplateRequest tinha
// perdido o campo Buttons, sem o qual a capability não tem sentido — um
// template hidratado sem botão é uma mensagem de texto com rodapé —, então
// recuperá-la exigiu ACRESCENTAR schema ao request.
//
// Este use case não conhece protobuf: domain.TemplateButton é DTO de
// domínio, e qual `waE2E.Hydrated*Button` cada um vira (com a numeração
// automática dos que não trazem ID) é tradução de port.SimpleMessenger.
type SendTemplateUseCase struct {
	messages appport.SimpleMessenger
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewSendTemplateUseCase cria uma nova instância do usecase.
func NewSendTemplateUseCase(sm appport.SimpleMessenger, jr appport.JIDResolver, l appport.Logger) *SendTemplateUseCase {
	return &SendTemplateUseCase{
		messages: sm,
		jids:     jr,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário e envia o
// template pela porta de verdade.
//
// A validação é a HISTÓRICA, na mesma ordem (`git show 41bc8e2^:handlers.go`,
// função SendTemplate): Phone ausente, Content ausente, Footer ausente,
// menos de um botão, e por fim o Phone que não parseia. Não há validação do
// CONTEÚDO de cada botão — o histórico nunca a teve, e um botão de url sem
// Url produzia lá o mesmo protobuf com string vazia que produz aqui.
// Acrescentá-la seria mudança de contrato público, não recuperação da
// capability.
//
// Phone é resolvido com ResolveJID, e não com ResolveQualifiedJID, porque o
// histórico o passava por parseJID (`41bc8e2^:handlers.go`, linha 3157), que
// aplica o servidor padrão a um número sem servidor. Trocar por
// ResolveQualifiedJID passaria a rejeitar entradas que a rota hoje aceita.
func (uc *SendTemplateUseCase) Execute(ctx context.Context, txtID string, req domain.SendTemplateRequest) (*domain.SendTemplateResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Content == "" {
		return nil, apperr.New("missing_content", apperr.CategoryValidation, "missing Content in payload", false, nil)
	}
	if req.Footer == "" {
		return nil, apperr.New("missing_footer", apperr.CategoryValidation, "missing Footer in payload", false, nil)
	}
	if len(req.Buttons) < 1 {
		return nil, apperr.New("missing_buttons", apperr.CategoryValidation, "missing Buttons in payload", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send template payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	payload := domain.TemplatePayload{
		Content: req.Content,
		Footer:  req.Footer,
		Buttons: req.Buttons,
	}

	sent, err := uc.messages.SendTemplate(ctx, txtID, recipient, payload, req.ReplyTo, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send template message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendTemplateResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "template sent", "msgID", result.MessageID)
	return result, nil
}
