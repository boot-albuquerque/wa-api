package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/util/dbutil"
)

const (
	getSessionQuery             = `SELECT session FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id=$2`
	hasSessionQuery             = `SELECT true FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id=$2`
	getManySessionQueryPostgres = `SELECT their_id, session FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id = ANY($2)`
	getManySessionQueryGeneric  = `SELECT their_id, session FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id IN (%s)`
	putSessionQuery             = `
		INSERT INTO whatsmeow_sessions (our_jid, their_id, session) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, their_id) DO UPDATE SET session=excluded.session
	`
	deleteAllSessionsQuery = `DELETE FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id LIKE $2`
	deleteSessionQuery     = `DELETE FROM whatsmeow_sessions WHERE our_jid=$1 AND their_id=$2`
)

func (s *SQLStore) GetSession(ctx context.Context, address string) (session []byte, err error) {
	err = s.db.QueryRow(ctx, getSessionQuery, s.JID, address).Scan(&session)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (s *SQLStore) HasSession(ctx context.Context, address string) (has bool, err error) {
	err = s.db.QueryRow(ctx, hasSessionQuery, s.JID, address).Scan(&has)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

type addressSessionTuple struct {
	Address string
	Session []byte
}

var sessionScanner = dbutil.ConvertRowFn[addressSessionTuple](func(row dbutil.Scannable) (out addressSessionTuple, err error) {
	err = row.Scan(&out.Address, &out.Session)
	return
})

func (s *SQLStore) GetManySessions(ctx context.Context, addresses []string) (map[string][]byte, error) {
	if len(addresses) == 0 {
		return nil, nil
	}

	var rows dbutil.Rows
	var err error
	if s.db.Dialect == dbutil.Postgres && PostgresArrayWrapper != nil {
		rows, err = s.db.Query(ctx, getManySessionQueryPostgres, s.JID, PostgresArrayWrapper(addresses))
	} else {
		args := make([]any, len(addresses)+1)
		placeholders := make([]string, len(addresses))
		args[0] = s.JID
		for i, addr := range addresses {
			args[i+1] = addr
			placeholders[i] = fmt.Sprintf("$%d", i+2)
		}
		rows, err = s.db.Query(ctx, fmt.Sprintf(getManySessionQueryGeneric, strings.Join(placeholders, ",")), args...)
	}
	result := make(map[string][]byte, len(addresses))
	for _, addr := range addresses {
		result[addr] = nil
	}
	err = sessionScanner.NewRowIter(rows, err).Iter(func(tuple addressSessionTuple) (bool, error) {
		result[tuple.Address] = tuple.Session
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *SQLStore) PutManySessions(ctx context.Context, sessions map[string][]byte) error {
	return s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		for addr, sess := range sessions {
			err := s.PutSession(ctx, addr, sess)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SQLStore) PutSession(ctx context.Context, address string, session []byte) error {
	_, err := s.db.Exec(ctx, putSessionQuery, s.JID, address, session)
	return err
}

func (s *SQLStore) DeleteAllSessions(ctx context.Context, phone string) error {
	return s.deleteAllSessions(ctx, phone)
}

func (s *SQLStore) deleteAllSessions(ctx context.Context, phone string) error {
	_, err := s.db.Exec(ctx, deleteAllSessionsQuery, s.JID, phone+signalAddressWildcardSuffix)
	return err
}

func (s *SQLStore) DeleteSession(ctx context.Context, address string) error {
	_, err := s.db.Exec(ctx, deleteSessionQuery, s.JID, address)
	return err
}
