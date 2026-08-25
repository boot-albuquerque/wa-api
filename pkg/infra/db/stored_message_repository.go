package db

import (
	"context"
	"database/sql"
	"fmt"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// StoredMessageRepository implements port.StoredMessageReader over *sqlx.DB.
//
// It reads the datajson column from message_history — the same blob written by
// SaveMessageToHistory (this file's sibling, message_history.go:23) via
// json.Marshal(evt) where evt is events.Message.
//
// The repository returns the raw datajson and the chat_jid. It does NOT
// deserialize the proto — that happens in the wa-noise adapter layer, which
// is the only layer allowed to import internal/wa-noise/protocol/proto.
type StoredMessageRepository struct {
	db *sqlx.DB
}

// NewStoredMessageRepository creates the repository.
func NewStoredMessageRepository(db *sqlx.DB) *StoredMessageRepository {
	return &StoredMessageRepository{db: db}
}

// GetStoredMessage implements port.StoredMessageReader.
//
// Production path: the datajson column is written by
// pkg/infra/db/message_history.go:23 (SaveMessageToHistory).
func (r *StoredMessageRepository) GetStoredMessage(ctx context.Context, userID, messageID string) (*domain.StoredMessageData, error) {
	query := r.db.Rebind(`SELECT chat_jid, datajson FROM message_history WHERE user_id = ? AND message_id = ? LIMIT 1`)

	var chatJID string
	var dataJSON sql.NullString
	err := r.db.QueryRowContext(ctx, query, userID, messageID).Scan(&chatJID, &dataJSON)
	switch {
	case err == sql.ErrNoRows:
		return nil, apperr.New(
			"message_not_found",
			apperr.CategoryNotFound,
			"message not found in history",
			false,
			nil,
		)
	case err != nil:
		log.Error().Err(err).Str("user_id", userID).Str("message_id", messageID).
			Msg("failed to look up stored message")
		return nil, fmt.Errorf("failed to look up stored message: %w", err)
	}

	if !dataJSON.Valid || dataJSON.String == "" {
		return nil, apperr.New(
			"message_no_data",
			apperr.CategoryNotFound,
			"stored message has no data",
			false,
			nil,
		)
	}

	return &domain.StoredMessageData{
		DataJSON: dataJSON.String,
		ChatJID:  chatJID,
	}, nil
}

// GetPollSenderJID reads the sender_jid from message_history for a given poll
// message. This is the wire form of the poll creator's JID — the authoritative
// source for key derivation in EncryptPollVote (F228).
func (r *StoredMessageRepository) GetPollSenderJID(ctx context.Context, userID, messageID string) (string, error) {
	query := r.db.Rebind(`SELECT sender_jid FROM message_history WHERE user_id = ? AND message_id = ? LIMIT 1`)

	var senderJID string
	err := r.db.QueryRowContext(ctx, query, userID, messageID).Scan(&senderJID)
	switch {
	case err == sql.ErrNoRows:
		return "", fmt.Errorf("poll message %s not found in history", messageID)
	case err != nil:
		log.Error().Err(err).Str("user_id", userID).Str("message_id", messageID).
			Msg("failed to look up poll sender")
		return "", fmt.Errorf("failed to look up poll sender: %w", err)
	}

	return senderJID, nil
}
