package db

import (
	"context"

	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// migrationIDUsersAccountType records, per session row, whether it is known
// to be personal or Business.
//
// Follows migration 19 (users.engine) exactly: a plain ALTER on a column
// with a NOT NULL DEFAULT, because both dialects accept it unchanged and the
// migrations table is what stops it running twice.
const migrationIDUsersAccountType = 21

const migrationNameUsersAccountType = "add_users_account_type"

const (
	// usersAccountTypeColumn is the column name (ADR-0004: named, not a
	// literal repeated across the DDL and every read/write).
	usersAccountTypeColumn = "account_type"
)

// addUsersAccountTypeSQL adds the column, defaulting every existing row to
// domain.AccountTypeUnknown.
//
// UNKNOWN AND NOT PERSONAL is the deliberate default, matching
// domain.AccountType's own doc: a row this ALTER has never measured must not
// silently claim to be a personal account. Detection (item 33/34 of the
// architectural prompt) is what promotes a row out of unknown, and it runs
// BEFORE authentication completes — see bootstrap wiring — so no session
// should stay at this default past its first successful pairing.
const addUsersAccountTypeSQL = `
ALTER TABLE users ADD COLUMN account_type TEXT NOT NULL DEFAULT '` + string(domain.AccountTypeUnknown) + `';
`

const addUsersAccountTypeDownSQL = `
ALTER TABLE users DROP COLUMN account_type;
`

// SetUserAccountType persists the detected account type for id.
//
// It is called after a successful detection (or explicitly with
// domain.AccountTypeUnknown before authentication — item 33), never with a
// classification the caller did not actually measure.
func SetUserAccountType(ctx context.Context, db *sqlx.DB, id string, accountType domain.AccountType) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET account_type = $1 WHERE id = $2`, accountType, id)
	if err != nil {
		log.Error().Err(err).Str("table", "users").Str("column", usersAccountTypeColumn).
			Str("user_id", id).Str("account_type", accountType.String()).
			Msg("failed to persist account type")
	}
	return err
}

// GetUserAccountType reads back the persisted account type for id.
//
// A row that does not exist is reported as domain.AccountTypeUnknown with
// sql.ErrNoRows wrapped by the driver — callers that need to distinguish
// "unknown because unmeasured" from "unknown because absent" already have
// EnsureSession/session lookups upstream of this call for that.
func GetUserAccountType(ctx context.Context, db *sqlx.DB, id string) (domain.AccountType, error) {
	var accountType string
	if err := db.GetContext(ctx, &accountType,
		`SELECT account_type FROM users WHERE id = $1`, id); err != nil {
		log.Warn().Err(err).Str("table", "users").Str("user_id", id).
			Msg("failed to read account type")
		return domain.AccountTypeUnknown, err
	}
	return domain.AccountType(accountType), nil
}
