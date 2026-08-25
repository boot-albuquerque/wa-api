package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

const defaultForwardingScore = uint32(1)

// SendForwardUseCase forwards a message (CAP-49 text, CAP-55 by key).
//
// Two paths:
//   - By content (CAP-49): Phone + Body → new text message marked forwarded.
//   - By key (CAP-55): Phone + MessageID → stored message re-sent with
//     forwarding context. ForwardingScore is DERIVED from the stored message
//     (incremented by 1), never caller-supplied.
type SendForwardUseCase struct {
	textSender appport.TextMessenger
	fwdSender  appport.ForwardedMessageSender
	storedMsgs appport.StoredMessageReader
	jids       appport.JIDResolver
	logger     appport.Logger
}

// NewSendForwardUseCase creates the use case. storedMsgs and fwdSender may
// be nil when the by-key path is not wired (backward compat during rollout).
func NewSendForwardUseCase(
	tm appport.TextMessenger,
	fwd appport.ForwardedMessageSender,
	smr appport.StoredMessageReader,
	jr appport.JIDResolver,
	l appport.Logger,
) *SendForwardUseCase {
	return &SendForwardUseCase{
		textSender: tm,
		fwdSender:  fwd,
		storedMsgs: smr,
		jids:       jr,
		logger:     l,
	}
}

func (uc *SendForwardUseCase) Execute(ctx context.Context, txtID string, req domain.SendForwardRequest) (*domain.SendForwardResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}

	if req.MessageID != "" {
		return uc.forwardByKey(ctx, txtID, req)
	}
	return uc.forwardByContent(ctx, txtID, req)
}

// forwardByContent is the original CAP-49 path: caller provides text body.
func (uc *SendForwardUseCase) forwardByContent(ctx context.Context, txtID string, req domain.SendForwardRequest) (*domain.SendForwardResult, error) {
	if req.Body == "" {
		return nil, apperr.New("missing_body", apperr.CategoryValidation, "missing Body in payload", false, nil)
	}

	if err := uc.textSender.EnsureSession(ctx, txtID); err != nil {
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

	sent, err := uc.textSender.SendText(ctx, txtID, recipient, req.Body, nil, req.ReplyTo, req.MentionedJID, forward, req.ID)
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

// forwardByKey is the CAP-55 path: caller provides MessageID, the use case
// looks up the stored message and re-sends it with forwarding context.
func (uc *SendForwardUseCase) forwardByKey(ctx context.Context, txtID string, req domain.SendForwardRequest) (*domain.SendForwardResult, error) {
	if err := uc.fwdSender.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send forward payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	stored, err := uc.storedMsgs.GetStoredMessage(ctx, txtID, req.MessageID)
	if err != nil {
		uc.logger.Warn(ctx, "stored message lookup failed", "txtID", txtID, "messageID", req.MessageID, "error", err)
		return nil, err
	}

	sent, err := uc.fwdSender.SendForwardedMessage(ctx, txtID, recipient, stored.DataJSON, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to forward stored message", "txtID", txtID, "messageID", req.MessageID, "error", err)
		return nil, err
	}

	result := &domain.SendForwardResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "forwarded stored message sent", "msgID", result.MessageID, "originalID", req.MessageID)
	return result, nil
}
