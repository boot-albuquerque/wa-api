package sqlstore

import (
	"context"
	"database/sql"
	"errors"
)

const (
	putIdentityQuery = `
		INSERT INTO wanoise_identity_keys (our_jid, their_id, identity) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_id) DO UPDATE SET identity=excluded.identity
	`
	deleteAllIdentitiesQuery = `DELETE FROM wanoise_identity_keys WHERE our_jid=$1 AND their_id LIKE $2`
	deleteIdentityQuery      = `DELETE FROM wanoise_identity_keys WHERE our_jid=$1 AND their_id=$2`
	getIdentityQuery         = `SELECT identity FROM wanoise_identity_keys WHERE our_jid=$1 AND their_id=$2`
)

func (s *SQLStore) PutIdentity(ctx context.Context, address string, key [curve25519KeyLength]byte) error {
	_, err := s.db.Exec(ctx, putIdentityQuery, s.JID, address, key[:])
	return err
}

func (s *SQLStore) DeleteAllIdentities(ctx context.Context, phone string) error {
	_, err := s.db.Exec(ctx, deleteAllIdentitiesQuery, s.JID, phone+signalAddressWildcardSuffix)
	return err
}

// DeleteIdentity remove a identidade de UM endereco Signal (<user>:<device>).
//
// Usa deleteIdentityQuery (igualdade). Usava deleteAllIdentitiesQuery, que e' a
// de LIKE — o efeito observavel coincidia, porque o endereco chega completo e
// sem curinga, entao o LIKE degenerava em igualdade. Mas deixava
// deleteIdentityQuery como constante morta e, pior, o comportamento passava a
// depender de nenhum chamador jamais passar um `%` ou `_` no endereco (F21 em
// HOUSEKEEP.md). O `_` do LIKE casa qualquer caractere: um endereco terminando
// em `_1` apagaria tambem `x1`, `y1`... A igualdade nao tem essa aresta.
func (s *SQLStore) DeleteIdentity(ctx context.Context, address string) error {
	_, err := s.db.Exec(ctx, deleteIdentityQuery, s.JID, address)
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
