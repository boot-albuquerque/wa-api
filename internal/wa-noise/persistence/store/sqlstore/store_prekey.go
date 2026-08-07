package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.mau.fi/util/dbutil"

	"wa-api/internal/wa-noise/security/keys"
)

const (
	getLastPreKeyIDQuery        = `SELECT MAX(key_id) FROM wanoise_pre_keys WHERE jid=$1`
	insertPreKeyQuery           = `INSERT INTO wanoise_pre_keys (jid, key_id, key, uploaded) VALUES ($1, $2, $3, $4)`
	getUnuploadedPreKeysQuery   = `SELECT key_id, key FROM wanoise_pre_keys WHERE jid=$1 AND uploaded=false ORDER BY key_id LIMIT $2`
	getPreKeyQuery              = `SELECT key_id, key FROM wanoise_pre_keys WHERE jid=$1 AND key_id=$2`
	deletePreKeyQuery           = `DELETE FROM wanoise_pre_keys WHERE jid=$1 AND key_id=$2`
	markPreKeysAsUploadedQuery  = `UPDATE wanoise_pre_keys SET uploaded=true WHERE jid=$1 AND key_id<=$2`
	getUploadedPreKeyCountQuery = `SELECT COUNT(*) FROM wanoise_pre_keys WHERE jid=$1 AND uploaded=true`
)

func (s *SQLStore) genOnePreKey(ctx context.Context, id uint32, markUploaded bool) (*keys.PreKey, error) {
	key := keys.NewPreKey(id)
	_, err := s.db.Exec(ctx, insertPreKeyQuery, s.JID, key.KeyID, key.Priv[:], markUploaded)
	return key, err
}

func (s *SQLStore) getNextPreKeyID(ctx context.Context) (uint32, error) {
	var lastKeyID sql.NullInt32
	err := s.db.QueryRow(ctx, getLastPreKeyIDQuery, s.JID).Scan(&lastKeyID)
	if err != nil {
		return 0, fmt.Errorf("failed to query next prekey ID: %w", err)
	}
	return uint32(lastKeyID.Int32) + 1, nil
}

func (s *SQLStore) GenOnePreKey(ctx context.Context) (*keys.PreKey, error) {
	s.preKeyLock.Lock()
	defer s.preKeyLock.Unlock()
	nextKeyID, err := s.getNextPreKeyID(ctx)
	if err != nil {
		return nil, err
	}
	return s.genOnePreKey(ctx, nextKeyID, true)
}

func (s *SQLStore) GetOrGenPreKeys(ctx context.Context, count uint32) ([]*keys.PreKey, error) {
	s.preKeyLock.Lock()
	defer s.preKeyLock.Unlock()

	res, err := s.db.Query(ctx, getUnuploadedPreKeysQuery, s.JID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to query existing prekeys: %w", err)
	}
	newKeys := make([]*keys.PreKey, count)
	var existingCount uint32
	for res.Next() {
		var key *keys.PreKey
		key, err = scanPreKey(res)
		if err != nil {
			return nil, err
		} else if key != nil {
			newKeys[existingCount] = key
			existingCount++
		}
	}

	if existingCount < uint32(len(newKeys)) {
		var nextKeyID uint32
		nextKeyID, err = s.getNextPreKeyID(ctx)
		if err != nil {
			return nil, err
		}
		for i := existingCount; i < count; i++ {
			newKeys[i], err = s.genOnePreKey(ctx, nextKeyID, false)
			if err != nil {
				return nil, fmt.Errorf("failed to generate prekey: %w", err)
			}
			nextKeyID++
		}
	}

	return newKeys, nil
}

func scanPreKey(row dbutil.Scannable) (*keys.PreKey, error) {
	var priv []byte
	var id uint32
	err := row.Scan(&id, &priv)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else if len(priv) != curve25519KeyLength {
		return nil, ErrInvalidLength
	}
	return &keys.PreKey{
		KeyPair: *keys.NewKeyPairFromPrivateKey(*(*[curve25519KeyLength]byte)(priv)),
		KeyID:   id,
	}, nil
}

func (s *SQLStore) GetPreKey(ctx context.Context, id uint32) (*keys.PreKey, error) {
	return scanPreKey(s.db.QueryRow(ctx, getPreKeyQuery, s.JID, id))
}

func (s *SQLStore) RemovePreKey(ctx context.Context, id uint32) error {
	_, err := s.db.Exec(ctx, deletePreKeyQuery, s.JID, id)
	return err
}

func (s *SQLStore) MarkPreKeysAsUploaded(ctx context.Context, upToID uint32) error {
	_, err := s.db.Exec(ctx, markPreKeysAsUploadedQuery, s.JID, upToID)
	return err
}

func (s *SQLStore) UploadedPreKeyCount(ctx context.Context) (count int, err error) {
	err = s.db.QueryRow(ctx, getUploadedPreKeyCountQuery, s.JID).Scan(&count)
	return
}
