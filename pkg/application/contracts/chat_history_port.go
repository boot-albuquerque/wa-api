package port

import (
	"context"
	"time"
)

// ChatHistoryMessage is the application-boundary representation of one
// persisted message of the local chat history.
//
// It duplicates, field by field, the shape of pkg/infra/db.HistoryMessage —
// deliberately, and the duplication is the point. The JSON tags below ARE the
// public wire contract of GET /chat/history (recovered from commit 3dafae0,
// handlers.go:5012), so the use case has to own a type it can guarantee.
// Depending on db.HistoryMessage would trade a contract risk for an
// application->infra dependency, and any future column added for persistence
// reasons would silently leak into the response.
//
// It is NOT domain.HistoryMessage (pkg/domain/entities.go:44): that type has
// zero uses and a shape (jid/from/body/direction/status) that never existed on
// the wire. See HOUSEKEEP F123.
type ChatHistoryMessage struct {
	ID              int       `json:"id"`
	UserID          string    `json:"user_id"`
	ChatJID         string    `json:"chat_jid"`
	SenderJID       string    `json:"sender_jid"`
	MessageID       string    `json:"message_id"`
	Timestamp       time.Time `json:"timestamp"`
	MessageType     string    `json:"message_type"`
	TextContent     string    `json:"text_content"`
	MediaLink       string    `json:"media_link"`
	QuotedMessageID string    `json:"quoted_message_id,omitempty"`
	DataJson        string    `json:"data_json"`
}

// ChatIndexEntry is one chat of the `chat_jid=index` listing: which chat, and
// when its most recent persisted message arrived.
//
// LastUpdated is a preformatted RFC3339Nano string and not a time.Time because
// the historical response formatted it that way, and the formatting exists to
// strip the Go monotonic-clock suffix that the sqlite driver hands back for an
// aggregated column (see parseFlexTimestamp in pkg/infra/db/message_history.go).
type ChatIndexEntry struct {
	ChatJID     string `json:"chat_jid"`
	LastUpdated string `json:"last_updated"`
}

// ChatHistoryReader is the persistence port of GetChatHistoryUseCase.
//
// All three methods are scoped by userID, without exception. The `index`
// listing in particular is a DELIBERATE divergence from the historical
// implementation, which queried message_history with no WHERE user_id and
// returned every tenant's chats to any authenticated caller (HOUSEKEEP F125).
// The scoping lives in the QUERY, not in a post-filter: a global read that is
// filtered afterwards has already passed the other tenants' data through logs,
// tracing and mapping, and does not survive a refactor.
type ChatHistoryReader interface {
	// ListChatMessages returns the newest `limit` messages of (userID,
	// chatJID), ordered by timestamp DESC. An empty chat is an empty slice,
	// never an error.
	ListChatMessages(ctx context.Context, userID, chatJID string, limit int) ([]ChatHistoryMessage, error)

	// ChatIndexByUser returns the caller's chats keyed by user id, newest
	// activity first. The map carries AT MOST the caller's own key; the
	// map-of-user-id shape is preserved because it is the historical wire
	// shape, not because more than one key can appear.
	ChatIndexByUser(ctx context.Context, userID string) (map[string][]ChatIndexEntry, error)

	// HistoryLimit reads the persisted `history` column of the user — the
	// revalidation step of the history gate, run when the cached userinfo
	// says 0.
	HistoryLimit(ctx context.Context, userID string) (int, error)
}
