package message

import (
	"context"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SendEditMessageUseCase edita uma mensagem própria de verdade: valida os
// campos obrigatórios, resolve o destinatário e manda a edição pelo
// wa-noise. Só devolve domain.StatusSent DEPOIS que o envio retorna
// sucesso — nunca antes. Até o CAP-10 este usecase devolvia
// Status="validated" sem editar nada.
type SendEditMessageUseCase struct {
	chats  appport.ChatMessenger
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewSendEditMessageUseCase cria uma nova instância do usecase.
func NewSendEditMessageUseCase(cm appport.ChatMessenger, jr appport.JIDResolver, l appport.Logger) *SendEditMessageUseCase {
	return &SendEditMessageUseCase{
		chats:  cm,
		jids:   jr,
		logger: l,
	}
}

// Execute valida os campos obrigatórios, resolve o destinatário e edita a
// mensagem pela porta de verdade.
//
// A ORDEM das validações é a do histórico (`git show 41bc8e2^:handlers.go`,
// linhas 2919, 2924, 2929 e 2936): Phone, Body, o parse do Phone e por
// último Id. A ordem é OBSERVÁVEL — com Phone inválido E Id ausente, a
// causa devolvida é a do parse ("could not parse Phone"), e não a do Id —,
// por isso tem teste próprio (TestSendEditMessage_PhoneParseRejectedBeforeMissingID).
//
// MessageID no resultado é o ID da mensagem EDITADA (req.ID), pela mesma
// razão de DeleteMessageUseCase.
func (uc *SendEditMessageUseCase) Execute(ctx context.Context, txtID string, req domain.SendEditMessageRequest) (*domain.SendEditMessageResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.Body == "" {
		return nil, apperr.New("missing_body", apperr.CategoryValidation, "missing Body in payload", false, nil)
	}

	if err := uc.chats.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in edit message payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	if req.ID == "" {
		return nil, apperr.New("missing_id", apperr.CategoryValidation, "missing Id in payload", false, nil)
	}

	var ctxInfo *domain.EditContextInfo
	if req.StanzaID != nil || req.Participant != nil || len(req.MentionedJID) > 0 {
		ctxInfo = &domain.EditContextInfo{MentionedJID: req.MentionedJID}
		if req.StanzaID != nil {
			ctxInfo.StanzaID = *req.StanzaID
		}
		if req.Participant != nil {
			ctxInfo.Participant = *req.Participant
		}
	}

	sent, err := uc.chats.EditMessage(ctx, txtID, recipient, req.ID, req.Body, ctxInfo)
	if err != nil {
		uc.logger.Error(ctx, "failed to edit message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendEditMessageResult{
		MessageID: req.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "message edit sent", "msgID", result.MessageID)
	return result, nil
}
