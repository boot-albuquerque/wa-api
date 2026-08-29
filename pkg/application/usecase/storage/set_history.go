package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// Result and error messages of this use case, as constants: the test that
// locks the contract asserts the SAME strings production answers (ADR-0004).
// Every string here is the historical text, not an invention of this phase:
// `41bc8e2^:handlers.go:6074`, `:6053` and `:6060` respectively.
const (
	historyConfiguredDetails = "History configured successfully"
	historyNegativeMsg       = "history cannot be negative"
	historySaveFailedMsg     = "failed to save history configuration"
	historyNegativeCode      = "invalid_history"
)

// SetHistoryUseCase writes the per-user message-history limit.
type SetHistoryUseCase struct {
	sessions appport.SessionGuard
	store    appport.HistoryConfigStore
	cache    appport.UserInfoHistoryCache
	logger   appport.Logger
}

// NewSetHistoryUseCase creates the use case.
func NewSetHistoryUseCase(
	sg appport.SessionGuard,
	store appport.HistoryConfigStore,
	cache appport.UserInfoHistoryCache,
	l appport.Logger,
) *SetHistoryUseCase {
	return &SetHistoryUseCase{sessions: sg, store: store, cache: cache, logger: l}
}

// Execute validates, writes and only then publishes to the cache.
//
// The ORDER is the contract, not an implementation detail:
//
//  1. session — the historical handler refused with "no session" before
//     reading anything (`41bc8e2^:handlers.go:6038`);
//  2. validate — a negative limit never reaches the database;
//  3. write — the source of truth first;
//  4. cache — LAST. Publishing before the write would show the user a limit
//     the database does not have, and the chat-history gate seeds itself from
//     exactly that cached value: it would then trust a number nothing backs.
//
// Step 4 is also what closes HOUSEKEEP F128. The gate revalidates against the
// users table whenever the cached History is 0, and the historical handler
// dropped the cache entry on the READ side so the next request would see the
// fresh value. Publishing here instead makes the gate read the right number on
// the FIRST request, one write instead of one invalidation per read, without
// carrying a token into the application layer.
func (uc *SetHistoryUseCase) Execute(ctx context.Context, txtID string, req domain.WebhookHistoryRequest) (*domain.WebhookHistoryResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if req.History < 0 {
		uc.logger.Warn(ctx, historyNegativeMsg, "txtID", txtID, "history", req.History)
		return nil, apperr.New(historyNegativeCode, apperr.CategoryValidation, historyNegativeMsg, false, nil)
	}

	if err := uc.store.SaveHistoryLimit(ctx, txtID, req.History); err != nil {
		uc.logger.Error(ctx, historySaveFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", historySaveFailedMsg, err)
	}

	uc.cache.SetHistory(ctx, txtID, req.History)

	uc.logger.Info(ctx, historyConfiguredDetails, "txtID", txtID, "history", req.History)
	return &domain.WebhookHistoryResult{Details: historyConfiguredDetails, History: req.History}, nil
}
