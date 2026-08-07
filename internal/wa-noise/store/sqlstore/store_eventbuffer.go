// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.mau.fi/util/dbutil"

	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
)

const (
	getBufferedEventQuery = `
		SELECT plaintext, server_timestamp, insert_timestamp FROM whatsmeow_event_buffer WHERE our_jid = $1 AND ciphertext_hash = $2
	`
	putBufferedEventQuery = `
		INSERT INTO whatsmeow_event_buffer (our_jid, ciphertext_hash, plaintext, server_timestamp, insert_timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`
	clearBufferedEventPlaintextQuery = `
		UPDATE whatsmeow_event_buffer SET plaintext = NULL WHERE our_jid = $1 AND ciphertext_hash = $2
	`
	deleteOldBufferedHashesQuery = `
		DELETE FROM whatsmeow_event_buffer WHERE insert_timestamp < $1
	`
)

func (s *SQLStore) GetBufferedEvent(ctx context.Context, ciphertextHash [ciphertextHashLength]byte) (*store.BufferedEvent, error) {
	var insertTimeMS, serverTimeSeconds int64
	var buf store.BufferedEvent
	err := s.db.QueryRow(ctx, getBufferedEventQuery, s.JID, ciphertextHash[:]).Scan(&buf.Plaintext, &serverTimeSeconds, &insertTimeMS)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	buf.ServerTime = time.Unix(serverTimeSeconds, 0)
	buf.InsertTime = time.UnixMilli(insertTimeMS)
	return &buf, nil
}

func (s *SQLStore) PutBufferedEvent(ctx context.Context, ciphertextHash [ciphertextHashLength]byte, plaintext []byte, serverTimestamp time.Time) error {
	_, err := s.db.Exec(ctx, putBufferedEventQuery, s.JID, ciphertextHash[:], plaintext, serverTimestamp.Unix(), time.Now().UnixMilli())
	return err
}

func (s *SQLStore) DoDecryptionTxn(ctx context.Context, fn func(context.Context) error) error {
	ctx = context.WithValue(ctx, dbutil.ContextKeyDoTxnCallerSkip, decryptionTxnCallerSkip)
	return s.db.DoTxn(ctx, nil, fn)
}

func (s *SQLStore) ClearBufferedEventPlaintext(ctx context.Context, ciphertextHash [ciphertextHashLength]byte) error {
	_, err := s.db.Exec(ctx, clearBufferedEventPlaintextQuery, s.JID, ciphertextHash[:])
	return err
}

func (s *SQLStore) DeleteOldBufferedHashes(ctx context.Context) error {
	// The WhatsApp servers only buffer events for 14 days,
	// so we can safely delete anything older than that.
	_, err := s.db.Exec(ctx, deleteOldBufferedHashesQuery, time.Now().Add(-bufferedEventRetention).UnixMilli())
	return err
}

const (
	getOutgoingEventQuery = `
		SELECT format, plaintext FROM whatsmeow_retry_buffer WHERE our_jid=$1 AND (chat_jid=$2 OR chat_jid=$3) AND message_id=$4
	`
	addOutgoingEventQuery = `
		INSERT INTO whatsmeow_retry_buffer (our_jid, chat_jid, message_id, format, plaintext, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (our_jid, chat_jid, message_id) DO UPDATE
			SET format=excluded.format, plaintext=excluded.plaintext, timestamp=excluded.timestamp
	`
	deleteOldOutgoingEventsQuery = `
		DELETE FROM whatsmeow_retry_buffer WHERE our_jid=$1 AND timestamp < $2
	`
)

func (s *SQLStore) GetOutgoingEvent(ctx context.Context, chatJID, altChatJID types.JID, id types.MessageID) (format string, result []byte, err error) {
	err = s.db.QueryRow(ctx, getOutgoingEventQuery, s.JID, chatJID, altChatJID, id).Scan(&format, &result)
	return
}

func (s *SQLStore) AddOutgoingEvent(ctx context.Context, chatJID types.JID, id types.MessageID, format string, plaintext []byte) error {
	_, err := s.db.Exec(ctx, addOutgoingEventQuery, s.JID, chatJID, id, format, plaintext, time.Now().UnixMilli())
	return err
}

func (s *SQLStore) DeleteOldOutgoingEvents(ctx context.Context) error {
	_, err := s.db.Exec(ctx, deleteOldOutgoingEventsQuery, s.JID, time.Now().Add(-outgoingEventRetention).UnixMilli())
	return err
}
