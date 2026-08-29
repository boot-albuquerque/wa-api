package sqlstore

import (
	"context"
	"fmt"

	"wa-api/internal/noise/protocol/types"
)

// Migracao de endereco Signal de PN (numero de telefone) para LID. As tres
// tabelas com chave por endereco Signal (sessoes, identity keys e sender keys)
// precisam ser reescritas juntas, na mesma transacao, senao uma sessao ficaria
// apontando para um endereco cuja identity key nao existe mais.
const (
	migratePNToLIDSessionsQuery = `
		INSERT INTO wanoise_sessions (our_jid, their_id, session)
		SELECT our_jid, replace(their_id, $2, $3), session
		FROM wanoise_sessions
		WHERE our_jid=$1 AND their_id LIKE $2 || ':%'
		ON CONFLICT (our_jid, their_id) DO UPDATE SET session=excluded.session
	`
	deleteAllIdentityKeysQuery      = `DELETE FROM wanoise_identity_keys WHERE our_jid=$1 AND their_id LIKE $2`
	migratePNToLIDIdentityKeysQuery = `
		INSERT INTO wanoise_identity_keys (our_jid, their_id, identity)
		SELECT our_jid, replace(their_id, $2, $3), identity
		FROM wanoise_identity_keys
		WHERE our_jid=$1 AND their_id LIKE $2 || ':%'
		ON CONFLICT (our_jid, their_id) DO UPDATE SET identity=excluded.identity
	`
	deleteAllSenderKeysQuery      = `DELETE FROM wanoise_sender_keys WHERE our_jid=$1 AND sender_id LIKE $2`
	migratePNToLIDSenderKeysQuery = `
		INSERT INTO wanoise_sender_keys (our_jid, chat_id, sender_id, sender_key)
		SELECT our_jid, chat_id, replace(sender_id, $2, $3), sender_key
		FROM wanoise_sender_keys
		WHERE our_jid=$1 AND sender_id LIKE $2 || ':%'
		ON CONFLICT (our_jid, chat_id, sender_id) DO UPDATE SET sender_key=excluded.sender_key
	`
)

func (s *SQLStore) deleteAllSenderKeys(ctx context.Context, phone string) error {
	_, err := s.db.Exec(ctx, deleteAllSenderKeysQuery, s.JID, phone+signalAddressWildcardSuffix)
	return err
}

func (s *SQLStore) deleteAllIdentityKeys(ctx context.Context, phone string) error {
	_, err := s.db.Exec(ctx, deleteAllIdentityKeysQuery, s.JID, phone+signalAddressWildcardSuffix)
	return err
}

func (s *SQLStore) MigratePNToLID(ctx context.Context, pn, lid types.JID) error {
	pnSignal := pn.SignalAddressUser()
	if !s.migratedPNSessionsCache.Add(pnSignal) {
		return nil
	}
	var sessionsUpdated, identityKeysUpdated, senderKeysUpdated int64
	lidSignal := lid.SignalAddressUser()
	err := s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		res, err := s.db.Exec(ctx, migratePNToLIDSessionsQuery, s.JID, pnSignal, lidSignal)
		if err != nil {
			return fmt.Errorf("failed to migrate sessions: %w", err)
		}
		sessionsUpdated, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected for sessions: %w", err)
		}
		err = s.deleteAllSessions(ctx, pnSignal)
		if err != nil {
			return fmt.Errorf("failed to delete extra sessions: %w", err)
		}

		res, err = s.db.Exec(ctx, migratePNToLIDIdentityKeysQuery, s.JID, pnSignal, lidSignal)
		if err != nil {
			return fmt.Errorf("failed to migrate identity keys: %w", err)
		}
		identityKeysUpdated, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected for identity keys: %w", err)
		}
		err = s.deleteAllIdentityKeys(ctx, pnSignal)
		if err != nil {
			return fmt.Errorf("failed to delete extra identity keys: %w", err)
		}

		res, err = s.db.Exec(ctx, migratePNToLIDSenderKeysQuery, s.JID, pnSignal, lidSignal)
		if err != nil {
			return fmt.Errorf("failed to migrate sender keys: %w", err)
		}
		senderKeysUpdated, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected for sender keys: %w", err)
		}
		err = s.deleteAllSenderKeys(ctx, pnSignal)
		if err != nil {
			return fmt.Errorf("failed to delete extra sender keys: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if sessionsUpdated > 0 || senderKeysUpdated > 0 || identityKeysUpdated > 0 {
		s.log.Infof("Migrated %d sessions, %d identity keys and %d sender keys from %s to %s", sessionsUpdated, identityKeysUpdated, senderKeysUpdated, pnSignal, lidSignal)
	} else {
		s.log.Debugf("No sessions or sender keys found to migrate from %s to %s", pnSignal, lidSignal)
	}
	return nil
}
