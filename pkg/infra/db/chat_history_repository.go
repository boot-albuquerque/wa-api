package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// chatHistoryColumns is the column list served by GET /chat/history.
//
// Named constant, not an inline literal, because the same list has to be the
// one the tests query — the ADR-0004 rule of this repository. `datajson` is
// the real column name; `data_json` is the JSON field, and the two differ.
const chatHistoryColumns = `id, user_id, chat_jid, sender_jid, message_id, timestamp,
	message_type, text_content, media_link, quoted_message_id, datajson`

// ChatHistoryRepository implements appport.ChatHistoryReader over *sqlx.DB.
//
// The SQL stays here, next to SaveMessageToHistory and TrimMessageHistory,
// which already own the message_history table. The use case never sees a
// query string.
type ChatHistoryRepository struct {
	db *sqlx.DB
}

// NewChatHistoryRepository creates the repository.
func NewChatHistoryRepository(db *sqlx.DB) *ChatHistoryRepository {
	return &ChatHistoryRepository{db: db}
}

// historyRow is the scan target. Nullable text columns are read as
// sql.NullString because rows written before the quoted_message_id/datajson
// migrations hold NULL, and scanning NULL into a string fails.
type historyRow struct {
	ID              int            `db:"id"`
	UserID          string         `db:"user_id"`
	ChatJID         string         `db:"chat_jid"`
	SenderJID       string         `db:"sender_jid"`
	MessageID       string         `db:"message_id"`
	Timestamp       time.Time      `db:"timestamp"`
	MessageType     string         `db:"message_type"`
	TextContent     sql.NullString `db:"text_content"`
	MediaLink       sql.NullString `db:"media_link"`
	QuotedMessageID sql.NullString `db:"quoted_message_id"`
	DataJson        sql.NullString `db:"datajson"`
}

// ListChatMessages implements appport.ChatHistoryReader.
//
// The WHERE is scoped by user_id AND chat_jid, and the ORDER BY is timestamp
// DESC — both recovered from the historical handler (commit 3dafae0,
// handlers.go:5012). db.Rebind handles the placeholder dialect, so the two
// backends share one query instead of the historical if/else that kept two
// copies of the same string in sync by hand.
func (r *ChatHistoryRepository) ListChatMessages(ctx context.Context, userID, chatJID string, limit int) ([]appport.ChatHistoryMessage, error) {
	query := r.db.Rebind(`SELECT ` + chatHistoryColumns + `
	                        FROM message_history
	                       WHERE user_id = ? AND chat_jid = ?
	                       ORDER BY timestamp DESC
	                       LIMIT ?`)

	var rows []historyRow
	if err := r.db.SelectContext(ctx, &rows, query, userID, chatJID, limit); err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Str("chat_jid", chatJID).Msg("failed to read chat history")
		return nil, fmt.Errorf("failed to get message history: %w", err)
	}

	// Non-nil even when empty: the wire contract is `[]`, and a nil slice
	// marshals to `null`. Absence of messages is not an error and must not
	// look like one to the caller.
	out := make([]appport.ChatHistoryMessage, 0, len(rows))
	for _, row := range rows {
		out = append(out, appport.ChatHistoryMessage{
			ID:              row.ID,
			UserID:          row.UserID,
			ChatJID:         row.ChatJID,
			SenderJID:       row.SenderJID,
			MessageID:       row.MessageID,
			Timestamp:       row.Timestamp,
			MessageType:     row.MessageType,
			TextContent:     row.TextContent.String,
			MediaLink:       row.MediaLink.String,
			QuotedMessageID: row.QuotedMessageID.String,
			DataJson:        row.DataJson.String,
		})
	}
	return out, nil
}

// ChatIndexByUser implements appport.ChatHistoryReader.
//
// WHERE user_id = ? is the tenant isolation, and it is IN THE QUERY. The
// historical implementation had no WHERE at all and returned every tenant's
// chats to any authenticated caller — a vulnerability, not a contract
// (HOUSEKEEP F125). The map-of-user-id response shape is preserved, so a
// client written against the old response keeps parsing; what changed is that
// the map now carries at most the caller's own key.
func (r *ChatHistoryRepository) ChatIndexByUser(ctx context.Context, userID string) (map[string][]appport.ChatIndexEntry, error) {
	query := r.db.Rebind(`SELECT user_id, chat_jid, MAX(timestamp) AS last_message_time
	                        FROM message_history
	                       WHERE user_id = ?
	                       GROUP BY user_id, chat_jid
	                       ORDER BY last_message_time DESC`)

	type indexRow struct {
		UserID   string       `db:"user_id"`
		ChatJID  string       `db:"chat_jid"`
		LastTime flexTimeScan `db:"last_message_time"`
	}
	var rows []indexRow
	if err := r.db.SelectContext(ctx, &rows, query, userID); err != nil {
		log.Error().Err(err).Str("table", "message_history").Str("user_id", userID).
			Msg("failed to read chat index")
		return nil, fmt.Errorf("failed to get chat mappings: %w", err)
	}

	// Empty map, not nil: the historical response for a user with no history
	// was `{}`. flexTimeScan (message_history.go) is reused because MAX() is
	// an aggregated column, and the sqlite driver hands those back as the Go
	// Stringer layout instead of RFC3339 — the same trap GetLastActivityByUser
	// already documents.
	out := make(map[string][]appport.ChatIndexEntry, len(rows))
	for _, row := range rows {
		if !row.LastTime.valid {
			log.Warn().Str("chat_jid", row.ChatJID).
				Msg("failed to parse chat index timestamp, skipping chat")
			continue
		}
		out[row.UserID] = append(out[row.UserID], appport.ChatIndexEntry{
			ChatJID:     row.ChatJID,
			LastUpdated: row.LastTime.t.Format(time.RFC3339Nano),
		})
	}
	return out, nil
}

// HistoryLimit implements appport.ChatHistoryReader — the revalidation half of
// the history gate.
//
// It reuses historyDaysQuery's shape (`SELECT COALESCE(history, 0) FROM users
// WHERE id = ?`), which is exactly what the historical handler re-ran when the
// cached userinfo said 0. A user row that is gone reads as 0, i.e. disabled:
// fail-closed, not fail-open.
func (r *ChatHistoryRepository) HistoryLimit(ctx context.Context, userID string) (int, error) {
	query := r.db.Rebind(`SELECT COALESCE(history, 0) FROM users WHERE id = ?`)

	var limit int
	err := r.db.GetContext(ctx, &limit, query, userID)
	switch {
	case err == nil:
		return limit, nil
	case errors.Is(err, sql.ErrNoRows):
		log.Warn().Str("table", "users").Str("user_id", userID).
			Msg("history revalidation found no user row; treating history as disabled")
		return 0, nil
	default:
		log.Error().Err(err).Str("table", "users").Str("user_id", userID).
			Msg("failed to revalidate history limit")
		return 0, fmt.Errorf("failed to read history limit: %w", err)
	}
}
