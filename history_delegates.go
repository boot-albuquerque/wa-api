package wuzapi

import (
	"context"

	"github.com/jmoiron/sqlx"
	"go.mau.fi/whatsmeow/types"
	wahistory "wuzapi/internal/infrastructure/history"
)

// syncHistoryForChat delegates to the history package via function closures.
func syncHistoryForChat(ctx context.Context, db *sqlx.DB, userID string, chatJID types.JID, count int) error {
	return wahistory.SyncHistoryForChat(ctx, db,
		wahistory.SyncDeps{
			GetWA: func(uid string) interface{} { return clientManager.GetWhatsmeowClient(uid) },
			GetMC: func(uid string) interface{} { return clientManager.GetMyClient(uid) },
		},
		userID, chatJID, count)
}

// saveOutgoingMessageToHistory delegates to the history package.
func saveOutgoingMessageToHistory(db *sqlx.DB, userID, chatJID, messageID, messageType, textContent, mediaLink string, historyLimit int) {
	wahistory.SaveOutgoingMessageToHistory(db,
		func(d *sqlx.DB, u, c, s, m, mt, t, ml, q, j string) error { return saveMessageToHistory(d, u, c, s, m, mt, t, ml, q, j) },
		func(d *sqlx.DB, u, c string, l int) error { return trimMessageHistory(d, u, c, l) },
		userID, chatJID, messageID, messageType, textContent, mediaLink, historyLimit)
}
