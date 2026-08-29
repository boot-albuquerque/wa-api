package db

import (
	"context"
	"fmt"
	"sort"

	"wa-api/pkg/domain"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// migrationIDUsersEngine records, per session row, which transport serves it.
//
// Until now the answer lived only in process configuration (WA_API_ENGINE and
// WA_API_ENGINE_HEADLESS_SESSIONS, read in pkg/bootstrap/engine_selection.go),
// which means no query could answer "what is this session running on?" and two
// replicas started with different environments disagreed silently.
const migrationIDUsersEngine = 19

const migrationNameUsersEngine = "add_users_engine"

const (
	usersTable = "users"

	// usersEngineColumn is the column name. It appears in the DDL, in the
	// backfill and in every read, so it is a constant (ADR-0004).
	usersEngineColumn = "engine"
)

// addUsersEngineSQL adds the column, and runs UNCHANGED on both SQLite and
// PostgreSQL.
//
// No information_schema guard and no separate SQLite branch, following
// migration 17 (addLeaseOwnerAddrSQL): a plain ALTER is valid in both dialects,
// and the migrations table is what stops it from running twice. A driver branch
// here would have to live in applyMigration's if/else chain, which the lint
// ratchet already caps at its current complexity — and the guard would buy
// nothing the migrations table does not already provide.
//
// NOT NULL with a DEFAULT is what makes this safe on a table that already has
// rows: without a default, SQLite refuses the statement outright. The default
// is legacy_unknown and NOT wa_noise on purpose — the ALTER must not claim to
// know something it did not measure. BackfillUserEngines is what replaces it,
// and it runs right after.
const addUsersEngineSQL = `
ALTER TABLE users ADD COLUMN engine TEXT NOT NULL DEFAULT '` + string(domain.EngineLegacyUnknown) + `';
`

const addUsersEngineDownSQL = `
ALTER TABLE users DROP COLUMN engine;
`

// EngineBackfillReport is what the backfill DID, so the operator can read the
// numbers instead of inferring them from the absence of an error.
type EngineBackfillReport struct {
	// TotalUsers is how many rows the users table held when the backfill ran.
	TotalUsers int

	// PendingBefore is how many of those rows still said legacy_unknown.
	PendingBefore int

	// ToHeadless and ToNoise are rows this run actually changed.
	ToHeadless int
	ToNoise    int

	// ListedButAbsent are ids present in WA_API_ENGINE_HEADLESS_SESSIONS that
	// matched no row. Reported and NOT an error: an operator may list a
	// session before creating it, and failing startup over it would be worse
	// than saying it.
	ListedButAbsent []string

	// RemainingLegacyUnknown must be zero after a successful run. It is
	// measured, not assumed — see ErrEngineBackfillIncomplete.
	RemainingLegacyUnknown int
}

// ErrEngineBackfillIncomplete is returned when rows would be left saying
// legacy_unknown after the backfill.
//
// legacy_unknown exists for robustness of the type, not as a steady state of
// this database. A row left in it is a session whose transport nothing can
// answer for, and discovering that later — from a routing failure — is strictly
// worse than discovering it here.
var ErrEngineBackfillIncomplete = fmt.Errorf(
	"engine backfill incomplete: rows remain at %q", domain.EngineLegacyUnknown)

// BackfillUserEngines writes users.engine for every row that does not have it
// yet, reproducing EXACTLY the rule that runs in production today:
//
//	id listed in WA_API_ENGINE_HEADLESS_SESSIONS -> wa_headless
//	everything else                              -> the default engine (wa_noise)
//
// # Why this is Go and not SQL
//
// The rule depends on an environment variable, which is an application
// decision, not a schema one. Putting it in the migration's UpSQL would mean
// the migration behaves differently depending on who ran it — the same reason
// migrations 11 and 16 already run in Go.
//
// # Why it is idempotent, and how
//
// It only touches rows still at legacy_unknown. Running it again is a no-op,
// which matters because it runs on EVERY startup, not once: there is no
// "already applied" flag to trust, and there must not be — once the API can set
// engine per session, a backfill that rewrote every row would silently undo
// that choice on the next restart.
//
// headlessSessionIDs are the ids explicitly placed on headless.
// defaultEngine is what every other row gets, and it must be valid for
// creation: a default of legacy_unknown would leave the table in the exact
// state this function exists to remove.
func BackfillUserEngines(
	ctx context.Context,
	db *sqlx.DB,
	headlessSessionIDs []string,
	defaultEngine domain.Engine,
) (EngineBackfillReport, error) {
	var report EngineBackfillReport

	if !defaultEngine.IsValidForCreate() {
		log.Error().Str("default_engine", defaultEngine.String()).
			Msg("engine backfill rejected: default engine is not valid for creation")
		return report, fmt.Errorf("%w (default engine %q)", domain.ErrInvalidEngine, defaultEngine)
	}

	if err := db.GetContext(ctx, &report.TotalUsers,
		`SELECT COUNT(*) FROM users`); err != nil {
		log.Error().Err(err).Str("table", usersTable).Str("query", "count_users").
			Msg("failed to count users before engine backfill")
		return report, err
	}
	if err := db.GetContext(ctx, &report.PendingBefore,
		`SELECT COUNT(*) FROM users WHERE engine = $1`, domain.EngineLegacyUnknown); err != nil {
		log.Error().Err(err).Str("table", usersTable).Str("query", "count_pending_engine").
			Msg("failed to count rows pending engine backfill")
		return report, err
	}

	// The headless ids go FIRST, and the order is load-bearing: the second
	// statement claims "everything still unknown", so running it first would
	// hand every listed session to the default engine and leave the first
	// statement with nothing to match.
	//
	// Sorted so that the log line and the report are stable across runs — map
	// iteration order in Go is randomized by design.
	ids := append([]string(nil), headlessSessionIDs...)
	sort.Strings(ids)

	headlessCount, absent, err := backfillHeadlessSessions(ctx, db, ids)
	if err != nil {
		return report, err
	}
	report.ToHeadless += headlessCount
	report.ListedButAbsent = absent

	res, err := db.ExecContext(ctx,
		`UPDATE users SET engine = $1 WHERE engine = $2`,
		defaultEngine, domain.EngineLegacyUnknown)
	if err != nil {
		log.Error().Err(err).Str("table", usersTable).Str("query", "backfill_engine_default").
			Str("default_engine", defaultEngine.String()).
			Msg("failed to set default engine during backfill")
		return report, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		log.Error().Err(err).Str("table", usersTable).
			Msg("failed to read rows affected for default engine backfill")
		return report, err
	}
	if defaultEngine == domain.EngineHeadless {
		report.ToHeadless += int(affected)
	} else {
		report.ToNoise += int(affected)
	}

	if err := db.GetContext(ctx, &report.RemainingLegacyUnknown,
		`SELECT COUNT(*) FROM users WHERE engine = $1`, domain.EngineLegacyUnknown); err != nil {
		log.Error().Err(err).Str("table", usersTable).Str("query", "count_remaining_engine").
			Msg("failed to verify engine backfill completion")
		return report, err
	}
	if report.RemainingLegacyUnknown > 0 {
		log.Error().Int("remaining", report.RemainingLegacyUnknown).
			Str("table", usersTable).Msg("engine backfill left rows without an engine")
		return report, ErrEngineBackfillIncomplete
	}

	return report, nil
}

// backfillHeadlessSessions sets wa_headless on each listed id that is still
// unrecorded, and reports which listed ids matched no row at all.
//
// Extracted from BackfillUserEngines so that neither function has to carry both
// the per-id bookkeeping and the whole-table sweep; the order between the two
// is what BackfillUserEngines keeps, and it is the load-bearing part.
func backfillHeadlessSessions(
	ctx context.Context,
	db *sqlx.DB,
	ids []string,
) (headlessCount int, absent []string, err error) {
	for _, id := range ids {
		res, err := db.ExecContext(ctx,
			`UPDATE users SET engine = $1 WHERE id = $2 AND engine = $3`,
			domain.EngineHeadless, id, domain.EngineLegacyUnknown)
		if err != nil {
			log.Error().Err(err).Str("table", usersTable).Str("user_id", id).
				Str("query", "backfill_engine_headless").
				Msg("failed to set headless engine during backfill")
			return 0, nil, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			log.Error().Err(err).Str("table", usersTable).Str("user_id", id).
				Msg("failed to read rows affected during engine backfill")
			return 0, nil, err
		}
		if affected > 0 {
			headlessCount += int(affected)
			continue
		}
		// Zero rows means either "no such session" or "already recorded".
		// Only the first is worth telling the operator about.
		var exists int
		if err := db.GetContext(ctx, &exists,
			`SELECT COUNT(*) FROM users WHERE id = $1`, id); err != nil {
			log.Error().Err(err).Str("table", usersTable).Str("user_id", id).
				Str("query", "user_exists_engine_backfill").
				Msg("failed to check session listed for headless")
			return 0, nil, err
		}
		if exists == 0 {
			absent = append(absent, id)
		}
	}

	return headlessCount, absent, nil
}
