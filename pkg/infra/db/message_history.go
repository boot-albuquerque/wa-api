package db

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// SaveMessageToHistory inserts a message into the message_history table.
// The insert is idempotent — duplicate (user_id, message_id) pairs are
// silently skipped via ON CONFLICT DO NOTHING (see #292).
//
// Moved from db_methods.go as part of Clean Architecture migration.
func SaveMessageToHistory(db *sqlx.DB, userID, chatJID, senderJID, messageID, messageType, textContent, mediaLink, quotedMessageID, dataJson string) error {
	query := db.Rebind(`INSERT INTO message_history (user_id, chat_jid, sender_jid, message_id, timestamp, message_type, text_content, media_link, quoted_message_id, datajson)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
              ON CONFLICT (user_id, message_id) DO NOTHING`)
	_, err := db.Exec(query, userID, chatJID, senderJID, messageID, time.Now(), messageType, textContent, mediaLink, quotedMessageID, dataJson)
	if err != nil {
		return fmt.Errorf("failed to save message to history: %w", err)
	}
	return nil
}

// TrimMessageHistory removes the oldest messages beyond the given limit for
// a (user_id, chat_jid) pair. Handles both PostgreSQL and SQLite drivers.
//
// Moved from db_methods.go as part of Clean Architecture migration.
func TrimMessageHistory(db *sqlx.DB, userID, chatJID string, limit int) error {
	var queryHistory, querySecrets string

	if db.DriverName() == "postgres" {
		queryHistory = `
	            DELETE FROM message_history
	            WHERE id IN (
	                SELECT id FROM message_history
	                WHERE user_id = $1 AND chat_jid = $2
	                ORDER BY timestamp DESC
	                OFFSET $3
	            )`

		querySecrets = `
	            DELETE FROM whatsmeow_message_secrets
	            WHERE message_id IN (
	                SELECT message_id FROM message_history
	                WHERE user_id = $1 AND chat_jid = $2
	                ORDER BY timestamp DESC
	                OFFSET $3
	            )`
	} else { // sqlite
		queryHistory = `
	            DELETE FROM message_history
	            WHERE id IN (
	                SELECT id FROM message_history
	                WHERE user_id = ? AND chat_jid = ?
	                ORDER BY timestamp DESC
	                LIMIT -1 OFFSET ?
	            )`

		querySecrets = `
	            DELETE FROM whatsmeow_message_secrets
	            WHERE message_id IN (
	                SELECT message_id FROM message_history
	                WHERE user_id = ? AND chat_jid = ?
	                ORDER BY timestamp DESC
	                LIMIT -1 OFFSET ?
	            )`
	}

	if _, err := db.Exec(querySecrets, userID, chatJID, limit); err != nil {
		return fmt.Errorf("failed to trim message secrets: %w", err)
	}

	if _, err := db.Exec(queryHistory, userID, chatJID, limit); err != nil {
		return fmt.Errorf("failed to trim message history: %w", err)
	}

	return nil
}
