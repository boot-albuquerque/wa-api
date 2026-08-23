package message

import (
	"context"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendPollUseCase envia uma enquete de verdade: monta o payload de criação a
// partir dos campos escalares do request (sem upload, sem fetch, sem
// conversão) e o envia pelo wa-noise. Só devolve domain.StatusSent depois
// que o envio retorna sucesso — nunca antes (mesma disciplina de
// SendMessageUseCase, CAP-01, e de SendLocationUseCase, CAP-08A).
//
// A memória do texto em claro das opções — sem a qual o voto que chegar
// depois é indecifrável, porque chega como SHA-256 do texto — é contrato de
// port.SimpleMessenger.SendPoll e efeito de INFRA. Este use case não a
// conhece: ele não pode conhecer o ClientManager, e o texto que precisa ser
// guardado já viaja no payload que ele entrega à porta.
type SendPollUseCase struct {
	messages appport.SimpleMessenger
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewSendPollUseCase cria uma nova instância do usecase.
func NewSendPollUseCase(sm appport.SimpleMessenger, jr appport.JIDResolver, l appport.Logger) *SendPollUseCase {
	return &SendPollUseCase{
		messages: sm,
		jids:     jr,
		logger:   l,
	}
}

// Execute valida os campos obrigatórios, resolve o grupo de destino e envia
// a enquete pela porta de verdade.
//
// A validação é a HISTÓRICA, na mesma ordem (`git show 41bc8e2^:handlers.go`,
// função SendPoll): Group ausente, Header ausente, menos de 2 opções. Não há
// validação de opção vazia nem de opção duplicada — o histórico nunca as
// teve, e acrescentá-las seria mudança de contrato público.
//
// Group é resolvido com ResolveJID, e não com ResolveQualifiedJID, porque o
// histórico o passava por ValidateMessageFields -> ParseJID
// (`41bc8e2^:internal/interfaces/http/handlers/common.go:132`), que aplica o
// servidor padrão a um número sem servidor. Trocar por ResolveQualifiedJID
// passaria a rejeitar entradas que a rota hoje aceita.
func (uc *SendPollUseCase) Execute(ctx context.Context, txtID string, req domain.SendPollRequest) (*domain.SendPollResult, error) {
	if req.Group == "" {
		return nil, apperr.New("missing_group", apperr.CategoryValidation, "missing Group in payload", false, nil)
	}
	if req.Header == "" {
		return nil, apperr.New("missing_header", apperr.CategoryValidation, "missing Header in payload", false, nil)
	}
	if len(req.Options) < 2 {
		return nil, apperr.New("insufficient_options", apperr.CategoryValidation, "at least 2 options are required", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Group)
	if err != nil {
		uc.logger.Warn(ctx, "invalid group in send poll payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_group", apperr.CategoryValidation, "could not parse Group", false, nil)
	}

	payload := domain.PollPayload{Name: req.Header, Options: req.Options}

	sent, err := uc.messages.SendPoll(ctx, txtID, recipient, payload, req.ReplyTo, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send poll message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendPollResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "poll sent", "msgID", result.MessageID)
	return result, nil
}
