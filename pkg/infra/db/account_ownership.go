package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// Account-identity ownership (feature/engine-session-ownership).
//
// session_leases (session_lease.go, ADR-0005) keys ownership by user_id
// alone, and only ever covered noise (F274). This file extends the SAME
// concept — "which process may serve this" — to a KEY that includes the
// engine, so the same phone number running noise AND headless
// simultaneously is two independent owners, not one contending pair. It does
// NOT touch session_leases: that table keeps deciding noise's per-process
// heartbeat/renewal (pkg/bootstrap/lease.go), a concern this table does not
// have. This table decides something session_leases never had to: WHICH
// authenticated claim, across possibly many for the same account+engine, is
// the current one — "newest wins" — and it keeps the LOSING claims as
// history (`superseded`), not merely as evicted rows.
//
// POSTGRES ONLY for the same reason as session_leases: `multi` mode requires
// Postgres (D1), and `single` mode has exactly one process, so there is
// nothing to arbitrate — the table exists in SQLite only so the two schemas
// do not diverge (see accountOwnershipSQLiteSQL).

// migrationIDAccountOwnership records the new table. Named because the
// dialect-routing branch in applyMigration and this file both reference it.
const migrationIDAccountOwnership = 20

const migrationNameAccountOwnership = "add_account_ownership"

// accountOwnershipStatusActive/Superseded are the two states a row can be in.
// Named constants per CLAUDE.md ("zero string literal solta"): both appear in
// SQL text and in Go comparisons, and a divergent literal in either would be
// exactly the kind of silent bug this file is not allowed to introduce.
const (
	accountOwnershipStatusActive     = "active"
	accountOwnershipStatusSuperseded = "superseded"
)

// accountOwnershipSQL creates the ownership table and its two indexes.
//
// The EXCLUSIVITY constraint is idx_account_ownership_active_unique: a
// PARTIAL unique index on (canonical_account_identity, engine) restricted to
// status='active'. This is the STRUCTURAL protection item 25/100 of the
// originating prompt asked for — it must reject a second active owner even
// when the application logic is bypassed entirely (raw SQL), which
// ClaimAccountIdentity's own transaction cannot guarantee on its own: the
// index is what turns "our Go code checks this" into "the database refuses
// this".
//
// ownership_revision is the fencing token (item 5): it increases by exactly
// one across the whole (identity, engine) lineage, active and superseded
// rows included, so a stale holder comparing its own revision against the
// CURRENT active row's can always tell it has been superseded — no clock,
// no wall-time comparison, immune to skew between replicas.
const accountOwnershipSQL = `
CREATE TABLE IF NOT EXISTS account_ownership (
    id                        TEXT PRIMARY KEY,
    canonical_account_identity TEXT        NOT NULL,
    engine                    TEXT        NOT NULL,
    session_id                TEXT        NOT NULL,
    owner_id                  TEXT        NOT NULL,
    status                    TEXT        NOT NULL,
    ownership_revision        BIGINT      NOT NULL,
    claimed_at                TIMESTAMPTZ NOT NULL,
    superseded_at             TIMESTAMPTZ,
    superseded_by_session_id  TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_account_ownership_active_unique
    ON account_ownership (canonical_account_identity, engine)
    WHERE status = '` + accountOwnershipStatusActive + `';
CREATE INDEX IF NOT EXISTS idx_account_ownership_identity_engine
    ON account_ownership (canonical_account_identity, engine, ownership_revision DESC);
`

// accountOwnershipSQLiteSQL is the same table without TIMESTAMPTZ, which
// SQLite does not know, mirroring session_leases' dialect split. SQLite DOES
// support partial unique indexes (the WHERE clause below is valid SQLite),
// so the structural constraint holds there too even though `single` mode
// never needs the arbitration ClaimAccountIdentity performs.
const accountOwnershipSQLiteSQL = `
CREATE TABLE IF NOT EXISTS account_ownership (
    id                        TEXT PRIMARY KEY,
    canonical_account_identity TEXT      NOT NULL,
    engine                    TEXT      NOT NULL,
    session_id                TEXT      NOT NULL,
    owner_id                  TEXT      NOT NULL,
    status                    TEXT      NOT NULL,
    ownership_revision        BIGINT    NOT NULL,
    claimed_at                TIMESTAMP NOT NULL,
    superseded_at             TIMESTAMP,
    superseded_by_session_id  TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_account_ownership_active_unique
    ON account_ownership (canonical_account_identity, engine)
    WHERE status = '` + accountOwnershipStatusActive + `';
CREATE INDEX IF NOT EXISTS idx_account_ownership_identity_engine
    ON account_ownership (canonical_account_identity, engine, ownership_revision DESC);
`

const accountOwnershipDownSQL = `DROP TABLE IF EXISTS account_ownership;`

// ErrOwnershipUnavailable separates "the claim lost, someone else is newer"
// (not an error, see ClaimAccountIdentity) from "the database could not be
// reached" — the same distinction session_lease.go draws for the same
// reason: a caller must not treat a network blip as "I am superseded".
var ErrOwnershipUnavailable = errors.New("database unavailable for account ownership operation")

// AccountOwnership is the current or historical state of one claim.
type AccountOwnership struct {
	ID                       string
	CanonicalAccountIdentity string
	Engine                   string
	SessionID                string
	OwnerID                  string
	Status                   string
	OwnershipRevision        int64
	ClaimedAt                time.Time
	SupersededAt             sql.NullTime
	SupersededBySessionID    sql.NullString
}

// IsActive reports whether this row is the current owner.
func (a AccountOwnership) IsActive() bool { return a.Status == accountOwnershipStatusActive }

// AccountOwnershipRepository manages the `account_ownership` table.
type AccountOwnershipRepository struct {
	db *sqlx.DB
}

func NewAccountOwnershipRepository(db *sqlx.DB) *AccountOwnershipRepository {
	return &AccountOwnershipRepository{db: db}
}

// claimAdvisoryLockSQL serializes concurrent claims for the SAME
// (canonical_account_identity, engine) pair, for the duration of the
// transaction (`pg_advisory_xact_lock` releases automatically on
// COMMIT/ROLLBACK — never held past this call).
//
// Why a lock instead of relying on the partial unique index alone: the
// index is the STRUCTURAL backstop (item 25) and is sufficient to guarantee
// at most one active row, but on its own it turns a lost race into a
// constraint-violation ERROR — the loser's transaction would abort instead
// of recording a coherent supersede. The lock makes the two concurrent
// claims run one after the other from the database's point of view, so the
// SECOND one to acquire the lock sees the first one's committed row and
// supersedes it in the open — which is what "newest wins" and the
// superseded_by bookkeeping (item 4) require. hashtextextended hashes the
// (identity, engine) pair into the 64-bit lock key; the constant salt (0)
// only has to be stable, not secret.
const claimAdvisoryLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended($1 || '::' || $2, 0))`

// currentActiveOwnerSQL reads the current active row for an identity+engine,
// if any, FOR UPDATE so it cannot change between the read and the supersede
// inside the same transaction — belt-and-suspenders alongside the advisory
// lock, and load-bearing for callers on a Postgres build old enough to lack
// hashtextextended (none in this repo's supported range, but the FOR UPDATE
// alone would already be correct; the lock exists to avoid the
// constraint-violation abort path described above).
const currentActiveOwnerSQL = `
SELECT id, canonical_account_identity, engine, session_id, owner_id, status,
       ownership_revision, claimed_at, superseded_at, superseded_by_session_id
  FROM account_ownership
 WHERE canonical_account_identity = $1 AND engine = $2 AND status = '` + accountOwnershipStatusActive + `'
 FOR UPDATE`

const supersedeSQL = `
UPDATE account_ownership
   SET status = '` + accountOwnershipStatusSuperseded + `',
       superseded_at = now(),
       superseded_by_session_id = $2
 WHERE id = $1`

const insertActiveSQL = `
INSERT INTO account_ownership
    (id, canonical_account_identity, engine, session_id, owner_id, status,
     ownership_revision, claimed_at)
VALUES ($1, $2, $3, $4, $5, '` + accountOwnershipStatusActive + `', $6, now())`

// ClaimAccountIdentity is the atomic, multi-replica-safe operation that
// decides who owns (canonicalAccountIdentity, engine).
//
// "Newest wins" (item 4): the caller of THIS call always wins — there is no
// notion of refusing a claim, only of recording who held it before. If
// another session currently holds an active claim for the same
// (identity, engine), it is marked `superseded` with `superseded_by_session_id`
// pointing at sessionID, and this claim becomes the new active row at
// ownershipRevision = previous + 1 (or 1 if there was none). Calling this
// again with the SAME sessionID for an identity+engine it already owns is a
// safe no-op renewal: it returns the same active row without bumping the
// revision or touching any superseded row, mirroring session_lease.go's
// Claim-doubles-as-renewal idempotence.
//
// claimID is caller-supplied (not generated here) so retries after a
// network timeout are idempotent at the row-identity level; callers should
// pass a fresh UUID per LOGICAL claim attempt, not per retry.
func (r *AccountOwnershipRepository) ClaimAccountIdentity(
	ctx context.Context,
	canonicalAccountIdentity, engine, sessionID, ownerID, claimID string,
) (AccountOwnership, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		log.Error().Err(err).Str("identity", canonicalAccountIdentity).Str("engine", engine).
			Str("session_id", sessionID).Str("query", "begin_claim_tx").
			Msg("failed to begin transaction for account ownership claim")
		return AccountOwnership{}, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, claimAdvisoryLockSQL, canonicalAccountIdentity, engine); err != nil {
		log.Error().Err(err).Str("identity", canonicalAccountIdentity).Str("engine", engine).
			Str("session_id", sessionID).Str("query", "claim_advisory_lock").
			Msg("failed to acquire advisory lock for account ownership claim")
		return AccountOwnership{}, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
	}

	var current AccountOwnership
	err = tx.QueryRowxContext(ctx, currentActiveOwnerSQL, canonicalAccountIdentity, engine).Scan(
		&current.ID, &current.CanonicalAccountIdentity, &current.Engine, &current.SessionID,
		&current.OwnerID, &current.Status, &current.OwnershipRevision, &current.ClaimedAt,
		&current.SupersededAt, &current.SupersededBySessionID)

	switch {
	case err == nil && current.SessionID == sessionID:
		// Renewal by the current owner: no supersede, no new row, no
		// revision bump. Committing (rather than just returning `current`)
		// is still required to release the advisory lock cleanly.
		if commitErr := tx.Commit(); commitErr != nil {
			log.Error().Err(commitErr).Str("identity", canonicalAccountIdentity).Str("engine", engine).
				Str("session_id", sessionID).Str("query", "commit_claim_renewal").
				Msg("failed to commit account ownership renewal")
			return AccountOwnership{}, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, commitErr)
		}
		return current, nil

	case err == nil:
		// Someone else is active for this identity+engine: supersede them.
		if _, err := tx.ExecContext(ctx, supersedeSQL, current.ID, sessionID); err != nil {
			log.Error().Err(err).Str("identity", canonicalAccountIdentity).Str("engine", engine).
				Str("superseded_session_id", current.SessionID).Str("new_session_id", sessionID).
				Str("query", "supersede_previous_owner").
				Msg("failed to supersede previous account owner")
			return AccountOwnership{}, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
		}

	case errors.Is(err, sql.ErrNoRows):
		// No current owner: nothing to supersede.

	default:
		log.Error().Err(err).Str("identity", canonicalAccountIdentity).Str("engine", engine).
			Str("session_id", sessionID).Str("query", "read_current_active_owner").
			Msg("failed to read current active owner during claim")
		return AccountOwnership{}, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
	}

	nextRevision := current.OwnershipRevision + 1
	claimedAt := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, insertActiveSQL,
		claimID, canonicalAccountIdentity, engine, sessionID, ownerID, nextRevision); err != nil {
		log.Error().Err(err).Str("identity", canonicalAccountIdentity).Str("engine", engine).
			Str("session_id", sessionID).Int64("next_revision", nextRevision).
			Str("query", "insert_active_owner").
			Msg("failed to insert new active account owner")
		return AccountOwnership{}, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
	}

	if err := tx.Commit(); err != nil {
		log.Error().Err(err).Str("identity", canonicalAccountIdentity).Str("engine", engine).
			Str("session_id", sessionID).Str("query", "commit_claim").
			Msg("failed to commit account ownership claim")
		return AccountOwnership{}, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
	}

	return AccountOwnership{
		ID:                       claimID,
		CanonicalAccountIdentity: canonicalAccountIdentity,
		Engine:                   engine,
		SessionID:                sessionID,
		OwnerID:                  ownerID,
		Status:                   accountOwnershipStatusActive,
		OwnershipRevision:        nextRevision,
		ClaimedAt:                claimedAt,
	}, nil
}

const currentBySessionSQL = `
SELECT id, canonical_account_identity, engine, session_id, owner_id, status,
       ownership_revision, claimed_at, superseded_at, superseded_by_session_id
  FROM account_ownership
 WHERE session_id = $1
 ORDER BY ownership_revision DESC
 LIMIT 1`

// CurrentStatusForSession reads the most recent ownership row for a session,
// regardless of engine or identity — this is the query the "token of a
// superseded session must fail" contract (item 6) is built on: a handler
// resolves the session_id carried by the token, calls this, and checks
// IsActive().
//
// found=false means this session_id never made a claim (fencing does not
// apply — it simply is not part of this mechanism, e.g. it predates this
// feature or is a different engine variant), which callers MUST NOT treat
// as "superseded".
func (r *AccountOwnershipRepository) CurrentStatusForSession(ctx context.Context, sessionID string) (AccountOwnership, bool, error) {
	var row AccountOwnership
	err := r.db.QueryRowxContext(ctx, currentBySessionSQL, sessionID).Scan(
		&row.ID, &row.CanonicalAccountIdentity, &row.Engine, &row.SessionID,
		&row.OwnerID, &row.Status, &row.OwnershipRevision, &row.ClaimedAt,
		&row.SupersededAt, &row.SupersededBySessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountOwnership{}, false, nil
		}
		log.Error().Err(err).Str("session_id", sessionID).Str("query", "current_status_for_session").
			Msg("failed to read ownership status for session")
		return AccountOwnership{}, false, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
	}
	return row, true, nil
}

const currentActiveSQL = `
SELECT id, canonical_account_identity, engine, session_id, owner_id, status,
       ownership_revision, claimed_at, superseded_at, superseded_by_session_id
  FROM account_ownership
 WHERE canonical_account_identity = $1 AND engine = $2 AND status = '` + accountOwnershipStatusActive + `'`

// CurrentActiveOwner reads the current active owner of (identity, engine)
// outside of a claim — used by fencing checks and by tests that assert the
// post-restart state directly against the database (item: "Restart" test),
// never against in-memory process state.
func (r *AccountOwnershipRepository) CurrentActiveOwner(ctx context.Context, canonicalAccountIdentity, engine string) (AccountOwnership, bool, error) {
	var row AccountOwnership
	err := r.db.QueryRowxContext(ctx, currentActiveSQL, canonicalAccountIdentity, engine).Scan(
		&row.ID, &row.CanonicalAccountIdentity, &row.Engine, &row.SessionID,
		&row.OwnerID, &row.Status, &row.OwnershipRevision, &row.ClaimedAt,
		&row.SupersededAt, &row.SupersededBySessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AccountOwnership{}, false, nil
		}
		log.Error().Err(err).Str("identity", canonicalAccountIdentity).Str("engine", engine).
			Str("query", "current_active_owner").
			Msg("failed to read current active owner")
		return AccountOwnership{}, false, fmt.Errorf("%w: %v", ErrOwnershipUnavailable, err)
	}
	return row, true, nil
}
