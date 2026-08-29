package message

import (
	"context"
	"fmt"
	"strings"
	"time"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ReactUseCase sends a reaction to a message
type ReactUseCase struct {
	chats  appport.ChatMessenger
	jids   appport.JIDResolver
	logger appport.Logger
}

// NewReactUseCase creates a new instance
func NewReactUseCase(cm appport.ChatMessenger, jr appport.JIDResolver, logger appport.Logger) *ReactUseCase {
	return &ReactUseCase{chats: cm, jids: jr, logger: logger}
}

// Execute sends a reaction
func (uc *ReactUseCase) Execute(ctx context.Context, userID string, req domain.ReactRequest) (*domain.SendReactionResult, error) {
	if err := uc.chats.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "error", err, "user_id", userID)
		return nil, err
	}

	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in Payload", false, nil)
	}

	if req.Body == "" {
		return nil, apperr.New("missing_body", apperr.CategoryValidation, "missing Body in Payload", false, nil)
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	msgid := req.Id
	if msgid == "" {
		return nil, apperr.New("missing_id", apperr.CategoryValidation, "missing Id in Payload", false, nil)
	}

	fromMe := false
	if strings.HasPrefix(msgid, "me:") {
		fromMe = true
		msgid = msgid[len("me:"):]
	}

	reaction := req.Body
	if reaction == "remove" {
		reaction = ""
	}

	// Um Participant que não resolve é ignorado, e não vira erro —
	// comportamento preservado do upstream.
	var participant domain.JID
	if !fromMe && req.Participant != "" {
		if pj, err := uc.jids.ResolveJID(ctx, req.Participant); err == nil {
			participant = pj
		}
	}

	resp, err := uc.chats.SendReaction(ctx, userID, recipient, domain.Reaction{
		TargetMessageID: msgid,
		FromMe:          fromMe,
		Participant:     participant,
		Text:            reaction,
		SentAt:          time.Now(),
	})
	if err != nil {
		uc.logger.Error(ctx, "Error sending reaction", "error", err, "user_id", userID)
		return nil, fmt.Errorf("error sending message: %v", err)
	}

	uc.logger.Info(ctx, "Reaction sent", "timestamp", fmt.Sprintf("%v", resp.Timestamp), "id", msgid, "user_id", userID)

	// F190. Era um map literal com {Details, Timestamp, Id} — a forma HISTÓRICA
	// que a F131 descartou por escrito para toda a superfície de envio. A
	// reação envia uma mensagem e devolve os mesmos três valores semânticos que
	// as catorze irmãs; devolvê-los com outros nomes obrigava um cliente a ter
	// dois parsers para a mesma coisa.
	//
	// O tipo é o que importa aqui, mais do que os nomes: sem ele não havia onde
	// pendurar a tag, e foi por isso que esta rota escapou à trava de wire por
	// tanto tempo.
	return &domain.SendReactionResult{
		MessageID: msgid,
		Timestamp: resp.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}, nil
}
