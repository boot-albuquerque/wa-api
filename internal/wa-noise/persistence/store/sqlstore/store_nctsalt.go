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
)

const (
	putNCTSaltQuery = `
		INSERT INTO whatsmeow_nct_salt (our_jid, salt) VALUES ($1, $2)
		ON CONFLICT (our_jid) DO UPDATE SET salt=excluded.salt
	`
	getNCTSaltQuery    = `SELECT salt FROM whatsmeow_nct_salt WHERE our_jid=$1`
	deleteNCTSaltQuery = `DELETE FROM whatsmeow_nct_salt WHERE our_jid=$1`
)

func (s *SQLStore) PutNCTSalt(ctx context.Context, salt []byte) error {
	_, err := s.db.Exec(ctx, putNCTSaltQuery, s.JID, salt)
	return err
}

func (s *SQLStore) GetNCTSalt(ctx context.Context) ([]byte, error) {
	var salt []byte
	err := s.db.QueryRow(ctx, getNCTSaltQuery, s.JID).Scan(&salt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return salt, nil
}

func (s *SQLStore) DeleteNCTSalt(ctx context.Context) error {
	_, err := s.db.Exec(ctx, deleteNCTSaltQuery, s.JID)
	return err
}
