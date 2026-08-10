package db

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// INTEGRATION test for the CONCURRENT claim of the webhook outbox (ADR-0005,
// D3), against a real Postgres.
//
// `webhook_outbox_test.go` says it in the open: the `FOR UPDATE SKIP LOCKED`
// branch (webhook_outbox.go:175-177) is not covered there, because those tests
// run on SQLite and the branch never even enters. That branch exists for one
// thing only — N concurrent sweepers must not claim the SAME delivery — and
// until this file it had never been exercised.
//
// `TestOutbox_ClaimIsExclusive` in the SQLite suite proves SEQUENTIAL
// exclusivity (one claim after the other). That is a different, weaker
// property: it holds even with no locking at all, because the second claim only
// sees the pushed `due_at` of the first. Real concurrency is what these tests
// add.
//
// Skipped when no database is configured. To run:
//
//	WA_API_TEST_POSTGRES="postgres://waapi:waapi@127.0.0.1:5433/medicao_m3?sslmode=disable" \
//	  go test ./pkg/infra/db/ -run TestOutboxPG -v
const truncateOutboxSQL = `DELETE FROM webhook_outbox`

// openTestOutboxPostgres follows the same convention as openTestPostgres in
// session_lease_integration_test.go: same env var, skip when unset, create the
// table, clean on the way in AND on the way out so a shared database is not
// polluted for the next measurement.
//
// It also HARD FAILS if the driver is not Postgres. Without that check the
// whole file could silently degrade into "ran against something else and passed
// anyway", which is exactly the failure mode this file exists to close.
func openTestOutboxPostgres(t *testing.T, maxConns int) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv(envTestPostgres)
	if dsn == "" {
		t.Skipf("no test Postgres: set %s (see the comment at the top of this file)", envTestPostgres)
	}
	database, err := sqlx.Connect(postgresDriver, dsn)
	if err != nil {
		t.Fatalf("connecting via %s: %v", envTestPostgres, err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// The claim opens one transaction per call; with fewer connections than
	// goroutines they would queue in the POOL instead of in the database, and the
	// test would be measuring Go, not SQL.
	database.SetMaxOpenConns(maxConns)
	database.SetMaxIdleConns(maxConns)

	if got := database.DriverName(); got != driverPostgres {
		t.Fatalf("driver = %q, want %q: the SKIP LOCKED branch in ClaimDue would not even be reached", got, driverPostgres)
	}
	var version string
	if err := database.Get(&version, `SELECT version()`); err != nil {
		t.Fatalf("SELECT version(): %v", err)
	}
	// Printed so the run log carries proof that it talked to a real server and
	// did not skip.
	t.Logf("connected to a REAL Postgres: %s", version)

	if _, err := database.Exec(addWebhookOutboxSQL); err != nil {
		t.Fatalf("creating the outbox table: %v", err)
	}
	if _, err := database.Exec(truncateOutboxSQL); err != nil {
		t.Fatalf("clearing the outbox: %v", err)
	}
	t.Cleanup(func() {
		if _, err := database.Exec(truncateOutboxSQL); err != nil {
			t.Logf("could not clear the outbox on the way out: %v", err)
		}
	})
	return database
}

// seedDueEntries inserts n entries already due, with STAGGERED `due_at`.
//
// The stagger is not cosmetic: `ORDER BY due_at` over ties gives each
// transaction an arbitrary order, and two transactions updating the same rows in
// opposite orders deadlock. A deadlock error would be indistinguishable from the
// bug under test, so the ordering is made total on purpose.
func seedDueEntries(t *testing.T, repo *WebhookOutboxRepository, n int) []string {
	t.Helper()
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("pg-e%04d", i)
		e := OutboxEntry{
			ID:      id,
			UserID:  "user-1",
			URL:     "https://example.invalid/hook",
			Payload: map[string]string{"type": "Message"},
			DueAt:   base.Add(time.Duration(i) * time.Millisecond),
		}
		if err := repo.Enqueue(ctx, e); err != nil {
			t.Fatalf("Enqueue %s: %v", id, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// TestOutboxPG_ConcurrentClaimIsDisjoint is the property the SKIP LOCKED branch
// exists for: N sweepers claiming AT THE SAME TIME split the queue into
// disjoint sets.
//
// If it did not hold, two replicas would deliver the same webhook twice — and
// the customer endpoint, which cannot tell a retry from a duplicate, would act
// on it twice.
//
// The assertion is on IDS, not on the count. A count alone would pass on a
// double-claim that also dropped a row, which is the more dangerous outcome of
// the two.
func TestOutboxPG_ConcurrentClaimIsDisjoint(t *testing.T) {
	const (
		total   = 200
		sweeper = 8
	)

	database := openTestOutboxPostgres(t, sweeper+2)
	repo := NewWebhookOutboxRepository(database)
	want := seedDueEntries(t, repo, total)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// All sweepers released at the same instant. Starting them as they are
	// spawned would let the first finish before the last begins, and the test
	// would quietly become the sequential one that already exists.
	start := make(chan struct{})
	var wg sync.WaitGroup
	claimed := make([][]string, sweeper)
	errs := make([]error, sweeper)

	for i := 0; i < sweeper; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			<-start
			// The batch is capped (claimBatch), so one call cannot drain the
			// queue; the sweeper loops until the queue answers "nothing due".
			for round := 0; round < 100; round++ {
				entries, err := repo.ClaimDue(ctx)
				if err != nil {
					errs[n] = fmt.Errorf("sweeper %d, round %d: %w", n, round, err)
					return
				}
				if len(entries) == 0 {
					return
				}
				for _, e := range entries {
					claimed[n] = append(claimed[n], e.ID)
				}
			}
			errs[n] = fmt.Errorf("sweeper %d never saw an empty queue", n)
		}(i)
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent claim failed: %v", err)
		}
	}

	seen := make(map[string]int, total)
	got := 0
	for n, ids := range claimed {
		got += len(ids)
		for _, id := range ids {
			seen[id]++
			if seen[id] > 1 {
				t.Errorf("%s was claimed %dx (last by sweeper %d): two replicas would deliver the same webhook", id, seen[id], n)
			}
		}
	}
	t.Logf("sweepers claimed: %v (total=%d, distinct=%d)", counts(claimed), got, len(seen))

	if got != total {
		t.Errorf("total claimed = %d, want %d", got, total)
	}
	if len(seen) != total {
		t.Errorf("distinct claimed = %d, want %d: an entry was left behind", len(seen), total)
	}
	for _, id := range want {
		if seen[id] == 0 {
			t.Errorf("%s was never claimed", id)
			break
		}
	}
}

func counts(claimed [][]string) []int {
	out := make([]int, len(claimed))
	for i, ids := range claimed {
		out[i] = len(ids)
	}
	return out
}

// TestOutboxPG_ClaimSkipsRowLockedElsewhere pins the SKIP LOCKED semantics
// DIRECTLY, without relying on a race to expose them.
//
// A row locked by someone else must be stepped over, not waited on. The three
// possible shapes of the query are told apart by this test:
//
//   - with FOR UPDATE SKIP LOCKED: returns immediately, without the locked row.
//   - with plain FOR UPDATE: blocks on the locked row until the context dies.
//   - with neither (the SQLite shape): reads the locked row, then blocks in the
//     UPDATE of the claim until the context dies.
//
// So a pass here is only reachable through the Postgres branch.
func TestOutboxPG_ClaimSkipsRowLockedElsewhere(t *testing.T) {
	database := openTestOutboxPostgres(t, 4)
	repo := NewWebhookOutboxRepository(database)
	ids := seedDueEntries(t, repo, 3)
	locked := ids[0] // the oldest: ORDER BY due_at would reach it FIRST.

	holder, err := database.Beginx()
	if err != nil {
		t.Fatalf("opening the holding transaction: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	var held string
	if err := holder.Get(&held, holder.Rebind(`SELECT id FROM webhook_outbox WHERE id = ? FOR UPDATE`), locked); err != nil {
		t.Fatalf("locking %s: %v", locked, err)
	}

	// A short deadline turns "waited for the lock" into a failure instead of a
	// hung test.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entries, err := repo.ClaimDue(ctx)
	if err != nil {
		t.Fatalf("ClaimDue waited on a row locked by someone else instead of skipping it: %v", err)
	}
	for _, e := range entries {
		if e.ID == locked {
			t.Fatalf("%s was claimed while another transaction held it locked", locked)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("claimed %d entries, want the 2 unlocked ones: %+v", len(entries), entries)
	}

	// And once the lock is gone the row is claimable again — SKIP LOCKED defers
	// the row, it does not drop it.
	if err := holder.Rollback(); err != nil {
		t.Fatalf("releasing the lock: %v", err)
	}
	entries, err = repo.ClaimDue(context.Background())
	if err != nil {
		t.Fatalf("ClaimDue after the lock was released: %v", err)
	}
	if len(entries) != 1 || entries[0].ID != locked {
		t.Fatalf("after the lock was released, claimed %+v, want only %s", entries, locked)
	}
}
