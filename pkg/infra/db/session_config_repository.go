package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// The four statements of the two configuration columns this repository owns,
// named.
//
// They are constants, and not inline literals, because the repository test has
// to execute THE SAME statement against the real schema — that is how F71
// escaped: a query naming a column that did not exist, exercised by no test.
// `?` is the portable placeholder; db.Rebind translates it to the dialect.
const (
	historyLimitUpdateQuery = "UPDATE users SET history = ? WHERE id = ?"
	historyLimitSelectQuery = "SELECT COALESCE(history, 0) FROM users WHERE id = ?"

	proxyConfigUpdateQuery  = "UPDATE users SET proxy_url = ?, webhook_use_proxy = ? WHERE id = ?"
	webhookUseProxySelect   = "SELECT COALESCE(webhook_use_proxy, true) FROM users WHERE id = ?"
	sessionConfigTableLabel = "users"
)

// defaultWebhookUseProxy is what LoadWebhookUseProxy reports for a user with
// no row. It is `true`, the same value the COALESCE above gives a NULL column
// and the same default the column was created with (`webhook_use_proxy
// BOOLEAN DEFAULT TRUE`, migrations.go:508).
const defaultWebhookUseProxy = true

// SessionConfigRepository implements appport.HistoryConfigStore and
// appport.ProxyConfigStore over *sqlx.DB.
//
// The two live in one type because they are one row: `POST /session/history`
// and `POST /session/proxy` write different columns of the same users record,
// and splitting them into two repositories would only duplicate the table
// label and the logging shape.
type SessionConfigRepository struct {
	db *sqlx.DB
}

// NewSessionConfigRepository creates the repository.
func NewSessionConfigRepository(db *sqlx.DB) *SessionConfigRepository {
	return &SessionConfigRepository{db: db}
}

// SaveHistoryLimit implements appport.HistoryConfigStore.
func (r *SessionConfigRepository) SaveHistoryLimit(ctx context.Context, userID string, history int) error {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(historyLimitUpdateQuery), history, userID); err != nil {
		log.Error().Err(err).Str("table", sessionConfigTableLabel).Str("user_id", userID).
			Str("query", "save_history_limit").Msg("failed to store the user history limit")
		return err
	}
	return nil
}

// LoadHistoryLimit implements appport.HistoryConfigStore — the read half of
// GET /webhook/history.
//
// The statement is the SAME shape the two other readers of this column already
// use (historyDaysQuery, pkg/bootstrap/user_info_cache.go:27, and
// ChatHistoryRepository.HistoryLimit, pkg/infra/db/chat_history_repository.go:153),
// so the number this route reports and the number the history gate enforces
// can never be two different readings of one column.
//
// A missing row reads as 0, i.e. history disabled — the same fail-closed
// answer the gate gives, and never an invented default.
func (r *SessionConfigRepository) LoadHistoryLimit(ctx context.Context, userID string) (int, error) {
	var limit int
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(historyLimitSelectQuery), userID).Scan(&limit)
	if errors.Is(err, sql.ErrNoRows) {
		log.Debug().Str("table", sessionConfigTableLabel).Str("user_id", userID).
			Msg("no user row while reading the history limit; reporting history as disabled")
		return 0, nil
	}
	if err != nil {
		log.Error().Err(err).Str("table", sessionConfigTableLabel).Str("user_id", userID).
			Str("query", "load_history_limit").Msg("failed to read the user history limit")
		return 0, err
	}
	return limit, nil
}

// SaveProxyConfig implements appport.ProxyConfigStore.
//
// One statement writes both columns, exactly as the historical UPDATE did
// (`41bc8e2^:handlers.go:6172`): the proxy URL and the flag that decides
// whether webhooks go through it must never be observable in disagreement.
func (r *SessionConfigRepository) SaveProxyConfig(ctx context.Context, userID, proxyURL string, webhookUseProxy bool) error {
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(proxyConfigUpdateQuery), proxyURL, webhookUseProxy, userID); err != nil {
		log.Error().Err(err).Str("table", sessionConfigTableLabel).Str("user_id", userID).
			Str("query", "save_proxy_config").Bool("enabling", proxyURL != "").
			Msg("failed to store the user proxy configuration")
		return err
	}
	return nil
}

// LoadWebhookUseProxy implements appport.ProxyConfigStore.
//
// A missing row reports the default instead of an error, for the same reason
// the historical handler ignored its Scan error: this read only decides a
// preference that the caller did not send, and turning its absence into a
// failure would refuse the proxy write itself.
func (r *SessionConfigRepository) LoadWebhookUseProxy(ctx context.Context, userID string) (bool, error) {
	var useProxy bool
	err := r.db.QueryRowxContext(ctx, r.db.Rebind(webhookUseProxySelect), userID).Scan(&useProxy)
	if errors.Is(err, sql.ErrNoRows) {
		log.Debug().Str("table", sessionConfigTableLabel).Str("user_id", userID).
			Msg("no user row while reading webhook_use_proxy; reporting the column default")
		return defaultWebhookUseProxy, nil
	}
	if err != nil {
		log.Error().Err(err).Str("table", sessionConfigTableLabel).Str("user_id", userID).
			Str("query", "load_webhook_use_proxy").Msg("failed to read the user webhook_use_proxy setting")
		return defaultWebhookUseProxy, err
	}
	return useProxy, nil
}
