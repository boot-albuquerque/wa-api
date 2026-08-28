package db

import (
	"context"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
)

// INTEGRATION test for account ownership (feature/engine-session-ownership),
// against a real Postgres — same reasoning as session_lease_integration_test.go:
// correctness lives in `pg_advisory_xact_lock` + `FOR UPDATE` + the partial
// unique index, none of which a double can imitate faithfully (ARMADILHAS #1).
//
// Skipped when no database is configured. To run:
//
//	docker compose -f infra/compose.yaml up -d postgres
//	WA_API_TEST_POSTGRES="postgres://waapi:waapi@127.0.0.1:5433/waapi?sslmode=disable" \
//	  go test ./pkg/infra/db/ -run TestAccountOwnership -race -v

const truncateAccountOwnershipSQL = `DELETE FROM account_ownership`

func openTestPostgresForOwnership(t *testing.T) *sqlx.DB {
	t.Helper()
	database := openTestPostgres(t) // reuses the skip-if-unset + cleanup logic
	if _, err := database.Exec(accountOwnershipSQL); err != nil {
		t.Fatalf("creating the account_ownership table: %v", err)
	}
	if _, err := database.Exec(truncateAccountOwnershipSQL); err != nil {
		t.Fatalf("clearing account_ownership: %v", err)
	}
	t.Cleanup(func() {
		if _, err := database.Exec(truncateAccountOwnershipSQL); err != nil {
			t.Logf("could not clear account_ownership on the way out: %v", err)
		}
	})
	return database
}

// TestAccountOwnership_Race is the central property (item "Race condition"):
// two sessions claiming the SAME (identity, engine) concurrently must leave
// EXACTLY one ACTIVE and the other SUPERSEDED. Run with -race.
func TestAccountOwnership_Race(t *testing.T) {
	repo := NewAccountOwnershipRepository(openTestPostgresForOwnership(t))
	ctx := context.Background()

	const identity, engine = "wa_pn:5511999999999", "noise"

	var wg sync.WaitGroup
	results := make([]AccountOwnership, 2)
	sessionIDs := []string{"session-A", "session-B"}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			row, err := repo.ClaimAccountIdentity(ctx, identity, engine, sessionIDs[i], "owner-"+sessionIDs[i], "claim-"+sessionIDs[i])
			if err != nil {
				t.Errorf("session %s: claim failed: %v", sessionIDs[i], err)
				return
			}
			results[i] = row
		}(i)
	}
	wg.Wait()

	active, ok, err := repo.CurrentActiveOwner(ctx, identity, engine)
	if err != nil {
		t.Fatalf("CurrentActiveOwner: %v", err)
	}
	if !ok {
		t.Fatal("no active owner after two concurrent claims: expected exactly one")
	}

	activeCount, supersededCount := 0, 0
	for _, sid := range sessionIDs {
		row, found, err := repo.CurrentStatusForSession(ctx, sid)
		if err != nil || !found {
			t.Fatalf("CurrentStatusForSession(%s): found=%v err=%v", sid, found, err)
		}
		if row.IsActive() {
			activeCount++
		} else {
			supersededCount++
		}
	}
	if activeCount != 1 || supersededCount != 1 {
		t.Fatalf("after the race: active=%d superseded=%d, want exactly 1 and 1", activeCount, supersededCount)
	}
	if active.SessionID != sessionIDs[0] && active.SessionID != sessionIDs[1] {
		t.Fatalf("active owner session_id = %q, not one of the two racers", active.SessionID)
	}
}

// TestAccountOwnership_CrossEngine (item "Cross-engine"): the SAME phone
// number running wa_noise and wa_headless simultaneously must NOT contend —
// the exclusivity key includes engine, and this proves the local constraint
// does not accidentally derrubar one by claiming the other.
func TestAccountOwnership_CrossEngine(t *testing.T) {
	repo := NewAccountOwnershipRepository(openTestPostgresForOwnership(t))
	ctx := context.Background()
	const identity = "wa_pn:5511988888888"

	noise, err := repo.ClaimAccountIdentity(ctx, identity, "noise", "session-noise", "owner-noise", "claim-noise")
	if err != nil {
		t.Fatalf("wa_noise claim: %v", err)
	}
	headless, err := repo.ClaimAccountIdentity(ctx, identity, "headless", "session-headless", "owner-headless", "claim-headless")
	if err != nil {
		t.Fatalf("wa_headless claim: %v", err)
	}

	noiseNow, ok, err := repo.CurrentStatusForSession(ctx, noise.SessionID)
	if err != nil || !ok || !noiseNow.IsActive() {
		t.Fatalf("wa_noise session got superseded by an unrelated wa_headless claim: row=%+v ok=%v err=%v", noiseNow, ok, err)
	}
	headlessNow, ok, err := repo.CurrentStatusForSession(ctx, headless.SessionID)
	if err != nil || !ok || !headlessNow.IsActive() {
		t.Fatalf("wa_headless session is not active: row=%+v ok=%v err=%v", headlessNow, ok, err)
	}
}

// TestAccountOwnership_Fencing (item "Fencing"): after B supersedes A, a
// delayed operation authenticated as A must be detectable as stale by
// comparing A's ownership_revision against the CURRENT active row's — never
// by re-claiming or otherwise touching state.
func TestAccountOwnership_Fencing(t *testing.T) {
	repo := NewAccountOwnershipRepository(openTestPostgresForOwnership(t))
	ctx := context.Background()
	const identity, engine = "wa_pn:5511977777777", "noise"

	a, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-A", "owner-A", "claim-A")
	if err != nil {
		t.Fatalf("A claim: %v", err)
	}
	b, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-B", "owner-B", "claim-B")
	if err != nil {
		t.Fatalf("B claim: %v", err)
	}
	if b.OwnershipRevision <= a.OwnershipRevision {
		t.Fatalf("B's revision (%d) must be greater than A's (%d): B superseded A", b.OwnershipRevision, a.OwnershipRevision)
	}

	// A "delayed callback" is simulated by re-reading the CURRENT active row
	// for the identity+engine and comparing it against the revision A was
	// holding onto — this is the exact check a fencing-aware caller performs
	// before applying any state mutation.
	current, ok, err := repo.CurrentActiveOwner(ctx, identity, engine)
	if err != nil || !ok {
		t.Fatalf("CurrentActiveOwner: ok=%v err=%v", ok, err)
	}
	if current.SessionID != b.SessionID {
		t.Fatalf("current owner = %q, want B (%q): A must not have been able to reassert ownership", current.SessionID, b.SessionID)
	}
	if current.OwnershipRevision == a.OwnershipRevision {
		t.Fatal("current revision equals A's stale revision: a delayed A callback would NOT be detected as fenced")
	}

	aStatus, _, err := repo.CurrentStatusForSession(ctx, "session-A")
	if err != nil {
		t.Fatalf("CurrentStatusForSession(A): %v", err)
	}
	if aStatus.IsActive() {
		t.Fatal("session A is still marked active after being superseded by B: fencing would never trigger")
	}
}

// TestAccountOwnership_Restart (item "Restart"): simulate a process restart
// by opening a FRESH repository/connection against the same database — never
// asserting against in-memory state — and confirming A stays superseded and
// B stays owner.
func TestAccountOwnership_Restart(t *testing.T) {
	database := openTestPostgresForOwnership(t)
	repo := NewAccountOwnershipRepository(database)
	ctx := context.Background()
	const identity, engine = "wa_pn:5511966666666", "noise"

	if _, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-A", "owner-A", "claim-A"); err != nil {
		t.Fatalf("A claim: %v", err)
	}
	if _, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-B", "owner-B", "claim-B"); err != nil {
		t.Fatalf("B claim: %v", err)
	}

	// "Restart": a brand-new repository value over a brand-new *sqlx.DB
	// handle, so nothing survives except what is in Postgres.
	restarted := NewAccountOwnershipRepository(openTestPostgres(t))

	aStatus, ok, err := restarted.CurrentStatusForSession(ctx, "session-A")
	if err != nil || !ok {
		t.Fatalf("post-restart CurrentStatusForSession(A): ok=%v err=%v", ok, err)
	}
	if aStatus.IsActive() {
		t.Fatal("post-restart: session A reads as active; ownership state did not survive in the database")
	}
	current, ok, err := restarted.CurrentActiveOwner(ctx, identity, engine)
	if err != nil || !ok {
		t.Fatalf("post-restart CurrentActiveOwner: ok=%v err=%v", ok, err)
	}
	if current.SessionID != "session-B" {
		t.Fatalf("post-restart active owner = %q, want session-B", current.SessionID)
	}
}

// TestAccountOwnership_StructuralConstraint (item "Proteção estrutural"):
// bypass the application entirely and try to INSERT a second active row for
// the same (identity, engine) via raw SQL. The database, not the Go code,
// must refuse it.
func TestAccountOwnership_StructuralConstraint(t *testing.T) {
	database := openTestPostgresForOwnership(t)
	repo := NewAccountOwnershipRepository(database)
	ctx := context.Background()
	const identity, engine = "wa_pn:5511955555555", "noise"

	if _, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-A", "owner-A", "claim-A"); err != nil {
		t.Fatalf("initial claim: %v", err)
	}

	// Negative control (ARMADILHAS #3): reintroduce a second ACTIVE owner
	// directly, bypassing ClaimAccountIdentity, and confirm the database
	// itself rejects it via the partial unique index.
	_, err := database.ExecContext(ctx, insertActiveSQL,
		"bypass-claim", identity, engine, "session-C", "owner-C", int64(999))
	if err == nil {
		t.Fatal("raw SQL inserted a second ACTIVE owner for the same (identity, engine): the partial unique index did not fire")
	}
	t.Logf("negative control confirmed: raw insert rejected by the database: %v", err)

	// Positive confirmation: exactly one active row remains, and it is the
	// original claim — the failed bypass must not have left partial state.
	active, ok, err := repo.CurrentActiveOwner(ctx, identity, engine)
	if err != nil || !ok {
		t.Fatalf("CurrentActiveOwner after rejected bypass: ok=%v err=%v", ok, err)
	}
	if active.SessionID != "session-A" {
		t.Fatalf("active owner after rejected bypass = %q, want session-A", active.SessionID)
	}
}

// TestAccountOwnership_RenewalIsIdempotent proves the "safe no-op renewal"
// documented on ClaimAccountIdentity: the SAME session claiming again does
// not bump the revision or create a superseded row for itself.
func TestAccountOwnership_RenewalIsIdempotent(t *testing.T) {
	repo := NewAccountOwnershipRepository(openTestPostgresForOwnership(t))
	ctx := context.Background()
	const identity, engine = "wa_pn:5511944444444", "noise"

	first, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-A", "owner-A", "claim-1")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := repo.ClaimAccountIdentity(ctx, identity, engine, "session-A", "owner-A", "claim-2")
	if err != nil {
		t.Fatalf("renewal claim: %v", err)
	}
	if second.OwnershipRevision != first.OwnershipRevision {
		t.Errorf("renewal changed the revision: %d -> %d, want unchanged", first.OwnershipRevision, second.OwnershipRevision)
	}

	var rowCount int
	if err := repo.db.GetContext(ctx, &rowCount,
		`SELECT COUNT(*) FROM account_ownership WHERE canonical_account_identity = $1 AND engine = $2`,
		identity, engine); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("row count after renewal = %d, want 1 (no superseded row created for self-renewal)", rowCount)
	}
}
