package message

import (
	"context"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DeleteMessageUseCase revoga ("apaga para todos") uma mensagem própria de
// verdade: valida os campos obrigatórios, resolve o destinatário e manda a
// revogação pelo wa-noise. Só devolve domain.StatusDeleted DEPOIS que o
// envio retorna sucesso — nunca antes (mesma disciplina de
// SendLocationUseCase, CAP-08A). Até o CAP-10 este usecase devolvia
// Status="validated" sem revogar nada.
type DeleteMessageUseCase struct {
	chats  appport.ChatMessenger
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewDeleteMessageUseCase cria uma nova instância do usecase.
func NewDeleteMessageUseCase(cm appport.ChatMessenger, jr appport.JIDResolver, l appport.Logger) *DeleteMessageUseCase {
	return &DeleteMessageUseCase{
		chats:  cm,
		jids:   jr,
		logger: l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário e revoga a
// mensagem pela porta de verdade.
//
// A ORDEM das validações é a do histórico (`git show 41bc8e2^:handlers.go`,
// linha 2825): Phone, depois Id, depois o parse do Phone.
//
// MessageID no resultado é o ID da mensagem REVOGADA (req.ID), não o da
// mensagem de revogação que o envio criou. É o que o histórico publicava
// (`"Id": msgid`) e é o único dos dois que responde à pergunta do cliente
// — "qual mensagem foi apagada". Divergência consciente das capabilities de
// ENVIO, onde MessageID é o id que o SDK usou para a mensagem nova; ver
// HOUSEKEEP F131 sobre a forma da resposta.
func (uc *DeleteMessageUseCase) Execute(ctx context.Context, txtID string, req domain.DeleteMessageRequest) (*domain.DeleteMessageResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.ID == "" {
		return nil, apperr.New("missing_id", apperr.CategoryValidation, "missing Id in payload", false, nil)
	}

	if err := uc.chats.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in delete message payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	sent, err := uc.chats.RevokeMessage(ctx, txtID, recipient, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to revoke message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.DeleteMessageResult{
		MessageID: req.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusDeleted,
	}

	uc.logger.Info(ctx, "message deleted", "msgID", result.MessageID)
	return result, nil
}
