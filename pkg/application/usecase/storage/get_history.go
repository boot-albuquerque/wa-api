package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// Message of the read failure of this use case, as a constant (ADR-0004).
//
// There is no historical text to reuse here, and that is worth saying out
// loud: `41bc8e2^:handlers.go:6497` never read history CONFIGURATION at all —
// the historical GetHistory read the message history (chat_jid, limit, the
// "index" mode and the 501 gate) and served BOTH /webhook/history and
// /chat/history. CAP-09A gave /chat/history the real implementation
// (HOUSEKEEP F124); this route's configuration-read contract is NEW, and the
// string "History configuration retrieved" it used to answer was an invention
// of the migration stub, not a contract (HOUSEKEEP F166).
const historyLoadFailedMsg = "failed to read history configuration"

// GetHistoryUseCase reads back the per-user message-history limit.
type GetHistoryUseCase struct {
	sessions appport.SessionGuard
	store    appport.HistoryConfigStore
	logger   appport.Logger
}

// NewGetHistoryUseCase creates the use case.
func NewGetHistoryUseCase(sg appport.SessionGuard, store appport.HistoryConfigStore, l appport.Logger) *GetHistoryUseCase {
	return &GetHistoryUseCase{sessions: sg, store: store, logger: l}
}

// Execute reports the limit stored in users.history.
//
// It reads the DATABASE and not a userinfo cache, deliberately. The caches are
// an authentication concern, they are published by the WRITE path
// (SetHistoryUseCase.Execute, step 4) and there are TWO of them with different
// keys and different readers (see appport.UserInfoHistoryCache). Answering a
// read from either would report whatever that cache last happened to hold,
// which is the class of defect of F128/F164.
//
// A read failure is a 500 with no value: reporting 0 on error would be
// indistinguishable from "history is disabled", the most common legitimate
// answer of this route.
func (uc *GetHistoryUseCase) Execute(ctx context.Context, txtID string) (*domain.WebhookHistoryResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	history, err := uc.store.LoadHistoryLimit(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, historyLoadFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", historyLoadFailedMsg, err)
	}

	uc.logger.Info(ctx, "history configuration read", "txtID", txtID, "history", history)
	return &domain.WebhookHistoryResult{History: history}, nil
}
