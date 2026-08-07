package sqlstore

import (
	"context"
	"database/sql"
	"errors"
)

const (
	getSenderKeyQuery = `SELECT sender_key FROM wanoise_sender_keys WHERE our_jid=$1 AND chat_id=$2 AND sender_id=$3`
	putSenderKeyQuery = `
		INSERT INTO wanoise_sender_keys (our_jid, chat_id, sender_id, sender_key) VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, chat_id, sender_id) DO UPDATE SET sender_key=excluded.sender_key
	`
)

func (s *SQLStore) PutSenderKey(ctx context.Context, group, user string, session []byte) error {
	_, err := s.db.Exec(ctx, putSenderKeyQuery, s.JID, group, user, session)
	return err
}

func (s *SQLStore) GetSenderKey(ctx context.Context, group, user string) (key []byte, err error) {
	err = s.db.QueryRow(ctx, getSenderKeyQuery, s.JID, group, user).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}
