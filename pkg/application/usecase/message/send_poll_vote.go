package message

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// SendPollVoteUseCase votes on an existing poll (CAP-48). The caller provides
// the four fields that identify the original poll message (Phone, Sender,
// PollMessageId, PollMessageTimestamp) plus the option names to vote for.
//
// Design choice (a): stateless, explicit. The caller always provides the full
// poll identity, and failures are always about what the caller provided.
type SendPollVoteUseCase struct {
	chat   appport.ChatMessenger
	jids   appport.JIDResolver
	logger appport.Logger
}

func NewSendPollVoteUseCase(cm appport.ChatMessenger, jr appport.JIDResolver, l appport.Logger) *SendPollVoteUseCase {
	return &SendPollVoteUseCase{chat: cm, jids: jr, logger: l}
}

func (uc *SendPollVoteUseCase) Execute(ctx context.Context, txtID string, req domain.SendPollVoteRequest) (*domain.SendPollVoteResult, error) {
	if req.Phone == "" {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in payload", false, nil)
	}
	if req.PollMessageID == "" {
		return nil, apperr.New("missing_poll_message_id", apperr.CategoryValidation, "missing PollMessageId in payload", false, nil)
	}
	if req.PollMessageTimestamp == 0 {
		return nil, apperr.New("missing_poll_message_timestamp", apperr.CategoryValidation, "missing PollMessageTimestamp in payload", false, nil)
	}
	if req.Sender == "" {
		return nil, apperr.New("missing_sender", apperr.CategoryValidation, "missing Sender in payload", false, nil)
	}
	if len(req.Options) == 0 {
		return nil, apperr.New("missing_options", apperr.CategoryValidation, "at least 1 option is required", false, nil)
	}

	if err := uc.chat.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send poll vote payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	senderJID, err := uc.jids.ResolveJID(ctx, req.Sender)
	if err != nil {
		uc.logger.Warn(ctx, "invalid sender in send poll vote payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_sender", apperr.CategoryValidation, "could not parse Sender", false, nil)
	}

	payload := domain.PollVotePayload{
		PollChat:      recipient,
		PollSender:    senderJID,
		PollMessageID: req.PollMessageID,
		PollTimestamp: req.PollMessageTimestamp,
		OptionNames:   req.Options,
	}

	sent, err := uc.chat.SendPollVote(ctx, txtID, recipient, payload, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send poll vote", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendPollVoteResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "poll vote sent", "msgID", result.MessageID)
	return result, nil
}
