package sqlstore

import (
	"context"
	"database/sql"
	"errors"
)

const (
	putIdentityQuery = `
		INSERT INTO whatsmeow_identity_keys (our_jid, their_id, identity) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_id) DO UPDATE SET identity=excluded.identity
	`
	deleteAllIdentitiesQuery = `DELETE FROM whatsmeow_identity_keys WHERE our_jid=$1 AND their_id LIKE $2`
	deleteIdentityQuery      = `DELETE FROM whatsmeow_identity_keys WHERE our_jid=$1 AND their_id=$2`
	getIdentityQuery         = `SELECT identity FROM whatsmeow_identity_keys WHERE our_jid=$1 AND their_id=$2`
)

func (s *SQLStore) PutIdentity(ctx context.Context, address string, key [curve25519KeyLength]byte) error {
	_, err := s.db.Exec(ctx, putIdentityQuery, s.JID, address, key[:])
	return err
}

func (s *SQLStore) DeleteAllIdentities(ctx context.Context, phone string) error {
	_, err := s.db.Exec(ctx, deleteAllIdentitiesQuery, s.JID, phone+signalAddressWildcardSuffix)
	return err
}

func (s *SQLStore) DeleteIdentity(ctx context.Context, address string) error {
	_, err := s.db.Exec(ctx, deleteAllIdentitiesQuery, s.JID, address)
	return err
}

func (s *SQLStore) IsTrustedIdentity(ctx context.Context, address string, key [curve25519KeyLength]byte) (bool, error) {
	var existingIdentity []byte
	err := s.db.QueryRow(ctx, getIdentityQuery, s.JID, address).Scan(&existingIdentity)
	if errors.Is(err, sql.ErrNoRows) {
		// Trust if not known, it'll be saved automatically later
		return true, nil
	} else if err != nil {
		return false, err
	} else if len(existingIdentity) != curve25519KeyLength {
		return false, ErrInvalidLength
	}
	return *(*[curve25519KeyLength]byte)(existingIdentity) == key, nil
}
