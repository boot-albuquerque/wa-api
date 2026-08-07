package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

const (
	putPrivacyTokens = `
		INSERT INTO whatsmeow_privacy_tokens (our_jid, their_jid, token, timestamp, sender_timestamp)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (our_jid, their_jid) DO UPDATE SET
			token=EXCLUDED.token,
			timestamp=EXCLUDED.timestamp,
			sender_timestamp=COALESCE(EXCLUDED.sender_timestamp, whatsmeow_privacy_tokens.sender_timestamp)
		WHERE EXCLUDED.timestamp >= whatsmeow_privacy_tokens.timestamp
	`
	// getPrivacyToken tambem casa o JID equivalente no outro espaco de
	// enderecamento (LID <-> PN), pelo mesmo motivo que getMsgSecret.
	getPrivacyToken = `
		SELECT token, timestamp, sender_timestamp FROM whatsmeow_privacy_tokens WHERE our_jid=$1 AND (their_jid=$2 OR their_jid=(
			CASE
				WHEN $2 LIKE '%@lid'
					THEN (SELECT pn || '@s.whatsapp.net' FROM whatsmeow_lid_map WHERE lid=replace($2, '@lid', ''))
				WHEN $2 LIKE '%@s.whatsapp.net'
					THEN (SELECT lid || '@lid' FROM whatsmeow_lid_map WHERE pn=replace($2, '@s.whatsapp.net', ''))
				ELSE $2
			END
		))
		ORDER BY timestamp DESC LIMIT 1
	`
	deleteExpiredPrivacyTokens = `
		DELETE FROM whatsmeow_privacy_tokens
		WHERE our_jid=$1 AND timestamp < $2
	`
)

func (s *SQLStore) PutPrivacyTokens(ctx context.Context, tokens ...store.PrivacyToken) error {
	args := make([]any, 1+len(tokens)*4)
	placeholders := make([]string, len(tokens))
	args[0] = s.JID
	for i, token := range tokens {
		args[i*4+1] = token.User.ToNonAD().String()
		args[i*4+2] = token.Token
		args[i*4+3] = token.Timestamp.Unix()
		if token.SenderTimestamp.IsZero() {
			args[i*4+4] = nil
		} else {
			args[i*4+4] = token.SenderTimestamp.Unix()
		}
		placeholders[i] = fmt.Sprintf("($1, $%d, $%d, $%d, $%d)", i*4+2, i*4+3, i*4+4, i*4+5)
	}
	query := strings.ReplaceAll(putPrivacyTokens, privacyTokenValuesTemplate, strings.Join(placeholders, ","))
	_, err := s.db.Exec(ctx, query, args...)
	return err
}

func (s *SQLStore) GetPrivacyToken(ctx context.Context, user types.JID) (*store.PrivacyToken, error) {
	var token store.PrivacyToken
	token.User = user.ToNonAD()
	var ts int64
	var senderTS sql.NullInt64
	err := s.db.QueryRow(ctx, getPrivacyToken, s.JID, token.User).Scan(&token.Token, &ts, &senderTS)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else {
		token.Timestamp = time.Unix(ts, 0)
		if senderTS.Valid {
			token.SenderTimestamp = time.Unix(senderTS.Int64, 0)
		}
		return &token, nil
	}
}

func (s *SQLStore) DeleteExpiredPrivacyTokens(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, deleteExpiredPrivacyTokens, s.JID, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return deleted, nil
}
