package sqlstore

import (
	"context"
	"database/sql"
	"errors"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

const (
	putMsgSecret = `
		INSERT INTO wanoise_message_secrets (our_jid, chat_jid, sender_jid, message_id, key)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (our_jid, chat_jid, sender_jid, message_id) DO NOTHING
	`
	// getMsgSecret busca tambem pelo JID equivalente do outro espaco de
	// enderecamento (LID <-> PN, via wa-noise_lid_map), porque o segredo pode
	// ter sido gravado antes ou depois da migracao do chat para LID.
	getMsgSecret = `
		SELECT key, sender_jid
		FROM wanoise_message_secrets
		WHERE our_jid=$1 AND (chat_jid=$2 OR chat_jid=(
			CASE
				WHEN $2 LIKE '%@lid'
					THEN (SELECT pn || '@s.whatsapp.net' FROM wanoise_lid_map WHERE lid=replace($2, '@lid', ''))
				WHEN $2 LIKE '%@s.whatsapp.net'
					THEN (SELECT lid || '@lid' FROM wanoise_lid_map WHERE pn=replace($2, '@s.whatsapp.net', ''))
			END
		)) AND message_id=$4 AND (sender_jid=$3 OR sender_jid=(
			CASE
				WHEN $3 LIKE '%@lid'
					THEN (SELECT pn || '@s.whatsapp.net' FROM wanoise_lid_map WHERE lid=replace($3, '@lid', ''))
				WHEN $3 LIKE '%@s.whatsapp.net'
					THEN (SELECT lid || '@lid' FROM wanoise_lid_map WHERE pn=replace($3, '@s.whatsapp.net', ''))
			END
		))
	`
)

func (s *SQLStore) PutMessageSecrets(ctx context.Context, inserts []store.MessageSecretInsert) (err error) {
	if len(inserts) == 0 {
		return nil
	}
	return s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		for _, insert := range inserts {
			_, err = s.db.Exec(ctx, putMsgSecret, s.JID, insert.Chat.ToNonAD(), insert.Sender.ToNonAD(), insert.ID, insert.Secret)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLStore) PutMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID, secret []byte) (err error) {
	_, err = s.db.Exec(ctx, putMsgSecret, s.JID, chat.ToNonAD(), sender.ToNonAD(), id, secret)
	return
}

func (s *SQLStore) GetMessageSecret(ctx context.Context, chat, sender types.JID, id types.MessageID) (secret []byte, realSender types.JID, err error) {
	err = s.db.QueryRow(ctx, getMsgSecret, s.JID, chat.ToNonAD(), sender.ToNonAD(), id).Scan(&secret, &realSender)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}
