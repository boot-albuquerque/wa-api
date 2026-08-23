package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

const defaultForwardingScore = uint32(1)

// SendForwardUseCase forwards a text message (CAP-49). The caller provides the
// text body and the API marks it as forwarded via ContextInfo.IsForwarded and
// ForwardingScore.
//
// Design choice (a): by content, stateless. Same reasoning as CAP-48
// (pollvote): no dependency on message_history retention.
type SendForwardUseCase struct {
	messages appport.TextMessenger
	jids     appport.JIDResolver
	logger   appport.Logger
}

func NewSendForwardUseCase(tm appport.TextMessenger, jr appport.JIDResolver, l appport.Logger) *SendForwardUseCase {
	return &SendForwardUseCase{messages: tm, jids: jr, logger: l}
}

func (uc *SendForwardUseCase) Execute(ctx context.Context, txtID string, req domain.SendForwardRequest) (*domain.SendForwardResult, error) {
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
		uc.logger.Warn(ctx, "invalid phone in send forward payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	score := defaultForwardingScore
	if req.ForwardingScore != nil && *req.ForwardingScore > 0 {
		score = *req.ForwardingScore
	}

	forward := &domain.ForwardContext{ForwardingScore: score}

	sent, err := uc.messages.SendText(ctx, txtID, recipient, req.Body, nil, req.ReplyTo, req.MentionedJID, forward, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send forwarded message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendForwardResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "forwarded message sent", "msgID", result.MessageID)
	return result, nil
}
