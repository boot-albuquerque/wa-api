package db

// Adversarial audit (feature/capability-final-audit) of invariant 3:
// for each (canonical_account_identity, engine) there is at most one
// ACTIVE owner session, even under split-brain concurrency with
// independent DB connections.
//
// See capability_audit_user_test.go (package db_test) for invariants 1
// and 2 (engine assignment and immutability).
//
// This file does not modify production code. Every test either RESISTS
// (passes, and is shown to actually exercise the rule) or documents a
// concrete break.

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
)

// ---------------------------------------------------------------------
// Invariant 3 — split-brain: independent DB connections racing for the
// same (canonical_account_identity, engine) must leave exactly one ACTIVE
// owner, on every repetition.
// ---------------------------------------------------------------------

// openIndependentTestPostgres opens a DEDICATED *sqlx.DB (its own
// connection pool, not a shared handle) so the two racing goroutines in
// TestAudit_Invariant3_SplitBrainManyRounds genuinely arrive as separate
// database connections/backends, mirroring two separate replicas - a
// shared *sql.DB with pooled connections would still let Go serialize
// query submission in ways a single TCP handshake per side avoids.
func openIndependentTestPostgres(t *testing.T) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv(envTestPostgres)
	if dsn == "" {
		t.Skipf("no test Postgres: set %s", envTestPostgres)
	}
	database, err := sqlx.Connect(postgresDriver, dsn)
	if err != nil {
		t.Fatalf("connecting via %s: %v", envTestPostgres, err)
	}
	// Force a real, separate physical connection immediately so the first
	// query in the race does not pay connection-establishment latency
	// unevenly between the two sides.
	if err := database.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// TestAudit_Invariant3_SplitBrainManyRounds runs the split-brain scenario
// (two independent DB connections claiming the SAME identity+engine at
// the same instant) 30 times, each round against a FRESH pair of session
// ids and a corner-shared advisory lock key, and demands EXACTLY one
// active + one superseded after every single round. A single lucky
// repetition would not distinguish "the mechanism works" from "the
// mechanism usually works" - CLAUDE.md's own measurement rule.
func TestAudit_Invariant3_SplitBrainManyRounds(t *testing.T) {
	const rounds = 30

	dbA := openIndependentTestPostgres(t)
	dbB := openIndependentTestPostgres(t)
	if _, err := dbA.Exec(accountOwnershipSQL); err != nil {
		t.Fatalf("creating account_ownership: %v", err)
	}
	if _, err := dbA.Exec(truncateAccountOwnershipSQL); err != nil {
		t.Fatalf("truncating account_ownership: %v", err)
	}
	t.Cleanup(func() {
		if _, err := dbA.Exec(truncateAccountOwnershipSQL); err != nil {
			t.Logf("cleanup truncate failed: %v", err)
		}
	})

	repoA := NewAccountOwnershipRepository(dbA)
	repoB := NewAccountOwnershipRepository(dbB)
	ctx := context.Background()

	type roundResult struct {
		round               int
		activeCount         int
		supersededCount     int
		activeSessionWins   string
		err                 error
	}

	failures := 0
	for round := 0; round < rounds; round++ {
		identity := fmt.Sprintf("wa_pn:audit-split-brain-%d", round)
		const engine = "wa_noise"
		sessionA := fmt.Sprintf("session-A-%d", round)
		sessionB := fmt.Sprintf("session-B-%d", round)

		var wg sync.WaitGroup
		var rowA, rowB AccountOwnership
		var errA, errB error
		wg.Add(2)
		go func() {
			defer wg.Done()
			rowA, errA = repoA.ClaimAccountIdentity(ctx, identity, engine, sessionA, "owner-A", "claim-A-"+sessionA)
		}()
		go func() {
			defer wg.Done()
			rowB, errB = repoB.ClaimAccountIdentity(ctx, identity, engine, sessionB, "owner-B", "claim-B-"+sessionB)
		}()
		wg.Wait()

		if errA != nil || errB != nil {
			t.Fatalf("round %d: claim errors errA=%v errB=%v", round, errA, errB)
		}
		_ = rowA
		_ = rowB

		activeCount, supersededCount := 0, 0
		var winner string
		for _, sid := range []string{sessionA, sessionB} {
			// Read via repoA - CurrentStatusForSession is a plain SELECT,
			// and reading through a third path (repoA, not the writer)
			// checks that the committed state is genuinely visible, not
			// an artifact of reading back through the same connection
			// that wrote it.
			row, found, err := repoA.CurrentStatusForSession(ctx, sid)
			if err != nil || !found {
				t.Fatalf("round %d: CurrentStatusForSession(%s) found=%v err=%v", round, sid, found, err)
			}
			if row.IsActive() {
				activeCount++
				winner = sid
			} else {
				supersededCount++
			}
		}

		res := roundResult{round: round, activeCount: activeCount, supersededCount: supersededCount, activeSessionWins: winner}
		if activeCount != 1 || supersededCount != 1 {
			failures++
			t.Errorf("round %d: SPLIT BRAIN - active=%d superseded=%d (want 1 and 1), winner=%q",
				res.round, res.activeCount, res.supersededCount, res.activeSessionWins)
			continue
		}

		// Also confirm the database-level structural backstop: at most one
		// row in status='active' for this identity+engine, queried
		// independently of CurrentStatusForSession.
		var activeRows int
		if err := dbA.Get(&activeRows,
			`SELECT COUNT(*) FROM account_ownership WHERE canonical_account_identity = $1 AND engine = $2 AND status = 'active'`,
			identity, engine); err != nil {
			t.Fatalf("round %d: counting active rows: %v", round, err)
		}
		if activeRows != 1 {
			failures++
			t.Errorf("round %d: %d active rows in account_ownership for the same identity+engine (partial unique index should forbid this)", round, activeRows)
		}
	}

	if failures > 0 {
		t.Fatalf("invariant 3 broke in %d/%d rounds", failures, rounds)
	}
	t.Logf("invariant 3 held across all %d split-brain rounds (independent DB connections)", rounds)
}

// TestAudit_Invariant3_StructuralIndexRejectsBypassingTheGoLayer attacks
// the invariant from BELOW the Go code entirely: it inserts two 'active'
// rows for the same (identity, engine) via raw SQL, bypassing
// ClaimAccountIdentity and its advisory lock completely, to confirm the
// partial unique index (the STRUCTURAL backstop the comment on
// accountOwnershipSQL claims) actually rejects the second insert rather
// than merely being aspirational documentation.
func TestAudit_Invariant3_StructuralIndexRejectsBypassingTheGoLayer(t *testing.T) {
	db := openIndependentTestPostgres(t)
	if _, err := db.Exec(accountOwnershipSQL); err != nil {
		t.Fatalf("creating account_ownership: %v", err)
	}
	if _, err := db.Exec(truncateAccountOwnershipSQL); err != nil {
		t.Fatalf("truncating: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(truncateAccountOwnershipSQL); err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	})

	const identity, engine = "wa_pn:audit-raw-sql-bypass", "wa_noise"
	insert := `INSERT INTO account_ownership
		(id, canonical_account_identity, engine, session_id, owner_id, status, ownership_revision, claimed_at)
		VALUES ($1, $2, $3, $4, $5, 'active', $6, now())`

	if _, err := db.Exec(insert, "row-1", identity, engine, "session-1", "owner-1", 1); err != nil {
		t.Fatalf("first raw insert should succeed: %v", err)
	}

	_, err := db.Exec(insert, "row-2", identity, engine, "session-2", "owner-2", 2)
	if err == nil {
		var activeRows int
		if getErr := db.Get(&activeRows,
			`SELECT COUNT(*) FROM account_ownership WHERE canonical_account_identity = $1 AND engine = $2 AND status = 'active'`,
			identity, engine); getErr != nil {
			t.Fatalf("counting rows: %v", getErr)
		}
		t.Fatalf("BROKE invariant 3 structurally: a second raw-SQL INSERT of an active row for the same "+
			"(identity, engine) succeeded with no error; %d active rows now exist. The partial unique index "+
			"idx_account_ownership_active_unique did not fire.", activeRows)
	}
	t.Logf("second raw INSERT correctly rejected by the partial unique index: %v", err)
}
