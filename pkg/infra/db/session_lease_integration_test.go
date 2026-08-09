package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// INTEGRATION test for the session lease (ADR-0005, D2), against a real
// Postgres.
//
// There is no useful unit version of this. Correctness lives entirely in the
// semantics of `ON CONFLICT (...) DO UPDATE ... WHERE`, and the question that
// decides the whole mechanism — "is the row left untouched and does
// RowsAffected return 0 when the WHERE fails?" — can only be answered by the
// server. A double would answer whatever I wrote into it, which is pitfall 1
// in ARMADILHAS.md.
//
// Skipped when no database is configured. To run:
//
//	docker compose -f infra/compose.yaml up -d postgres
//	WA_API_TEST_POSTGRES="postgres://waapi:waapi@127.0.0.1:5433/waapi?sslmode=disable" \
//	  go test ./pkg/infra/db/ -run TestLease -v

const (
	envTestPostgres  = "WA_API_TEST_POSTGRES"
	postgresDriver   = "postgres"
	truncateLeaseSQL = `DELETE FROM session_leases`
)

func openTestPostgres(t *testing.T) *sqlx.DB {
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

	if _, err := database.Exec(addSessionLeasesSQL); err != nil {
		t.Fatalf("creating the lease table: %v", err)
	}
	// Clean at the START so the outcome does not depend on execution order...
	if _, err := database.Exec(truncateLeaseSQL); err != nil {
		t.Fatalf("clearing leases: %v", err)
	}
	// ...and clean at the END so this suite does not leave rows behind in a
	// SHARED database. Cleaning only on entry is enough for the tests and wrong
	// for everyone else: leftover `pod-A`/`pod-B` rows once made a live
	// measurement look like `single` mode was writing leases, when it was not.
	// A test that pollutes shared state makes the next measurement lie.
	t.Cleanup(func() {
		if _, err := database.Exec(truncateLeaseSQL); err != nil {
			t.Logf("could not clear leases on the way out: %v", err)
		}
	})
	return database
}

// TestLease_SingleOwner is the central property: while one owner holds a valid
// lease, another cannot take it.
//
// Without this, two replicas connect the same session and we land in what F89
// measured — one `StreamReplaced` and the loser dead forever.
func TestLease_SingleOwner(t *testing.T) {
	repo := NewSessionLeaseRepository(openTestPostgres(t))
	ctx := context.Background()

	took, err := repo.Claim(ctx, "user-1", "pod-A", 30*time.Second)
	if err != nil {
		t.Fatalf("pod-A: %v", err)
	}
	if !took {
		t.Fatal("pod-A failed to take a free lease")
	}

	took, err = repo.Claim(ctx, "user-1", "pod-B", 30*time.Second)
	if err != nil {
		t.Fatalf("pod-B: %v", err)
	}
	if took {
		t.Fatal("pod-B took ownership while pod-A held a valid lease: both replicas would believe they own it")
	}

	owner, _, err := repo.CurrentOwner(ctx, "user-1")
	if err != nil {
		t.Fatalf("CurrentOwner: %v", err)
	}
	if owner != "pod-A" {
		t.Errorf("owner = %q, want pod-A", owner)
	}
}

// TestLease_RenewalBySameOwner: Claim doubles as renewal and must extend the
// deadline. If it did not, the lease would expire with the owner alive and
// another replica would take over a session that is actively being served.
func TestLease_RenewalBySameOwner(t *testing.T) {
	repo := NewSessionLeaseRepository(openTestPostgres(t))
	ctx := context.Background()

	if ok, err := repo.Claim(ctx, "user-2", "pod-A", 5*time.Second); err != nil || !ok {
		t.Fatalf("initial claim: ok=%v err=%v", ok, err)
	}
	_, firstExpiry, err := repo.CurrentOwner(ctx, "user-2")
	if err != nil {
		t.Fatalf("CurrentOwner: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	if ok, err := repo.Claim(ctx, "user-2", "pod-A", 30*time.Second); err != nil || !ok {
		t.Fatalf("renewal by the current owner was denied: ok=%v err=%v", ok, err)
	}
	_, secondExpiry, err := repo.CurrentOwner(ctx, "user-2")
	if err != nil {
		t.Fatalf("CurrentOwner: %v", err)
	}
	if !secondExpiry.After(firstExpiry) {
		t.Errorf("deadline was not extended: before=%s after=%s", firstExpiry, secondExpiry)
	}
}

// TestLease_ExpiredCanBeTaken: failover depends on this. If an expired lease
// could not be taken, a dead pod would hold the session hostage forever.
func TestLease_ExpiredCanBeTaken(t *testing.T) {
	repo := NewSessionLeaseRepository(openTestPostgres(t))
	ctx := context.Background()

	// A 1s TTL keeps the test from depending on a long sleep.
	if ok, err := repo.Claim(ctx, "user-3", "pod-A", 1*time.Second); err != nil || !ok {
		t.Fatalf("initial claim: ok=%v err=%v", ok, err)
	}
	if ok, _ := repo.Claim(ctx, "user-3", "pod-B", 30*time.Second); ok {
		t.Fatal("pod-B took the lease before it expired")
	}

	time.Sleep(1300 * time.Millisecond)

	ok, err := repo.Claim(ctx, "user-3", "pod-B", 30*time.Second)
	if err != nil {
		t.Fatalf("pod-B after expiry: %v", err)
	}
	if !ok {
		t.Fatal("pod-B could not take an EXPIRED lease: a dead pod would hold the session forever")
	}
}

// TestLease_ReleaseOnlyByOwner prevents the slow-shutdown accident: a pod that
// takes a while to die must not delete the lease the next replica already
// took, or both would believe they own the session.
func TestLease_ReleaseOnlyByOwner(t *testing.T) {
	repo := NewSessionLeaseRepository(openTestPostgres(t))
	ctx := context.Background()

	if ok, err := repo.Claim(ctx, "user-4", "pod-A", 30*time.Second); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	if err := repo.Release(ctx, "user-4", "pod-B"); err != nil {
		t.Fatalf("Release by a different owner returned an error (should be a no-op): %v", err)
	}
	owner, _, err := repo.CurrentOwner(ctx, "user-4")
	if err != nil {
		t.Fatalf("CurrentOwner: %v", err)
	}
	if owner != "pod-A" {
		t.Fatalf("pod-B deleted pod-A's lease; owner = %q", owner)
	}

	if err := repo.Release(ctx, "user-4", "pod-A"); err != nil {
		t.Fatalf("Release by the actual owner: %v", err)
	}
	owner, _, err = repo.CurrentOwner(ctx, "user-4")
	if err != nil {
		t.Fatalf("CurrentOwner after release: %v", err)
	}
	if owner != "" {
		t.Errorf("lease survived a release by its owner: %q", owner)
	}
}

// TestLease_ReleaseSpeedsUpFailover: after an explicit release another replica
// takes over immediately, without waiting out the TTL. That is what makes
// graceful shutdown cheap — F89 measured session re-establishment at 1.2s to
// 2.3s, so waiting 15s of TTL on a routine deploy would dominate the downtime.
func TestLease_ReleaseSpeedsUpFailover(t *testing.T) {
	repo := NewSessionLeaseRepository(openTestPostgres(t))
	ctx := context.Background()

	if ok, err := repo.Claim(ctx, "user-5", "pod-A", 300*time.Second); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err := repo.Release(ctx, "user-5", "pod-A"); err != nil {
		t.Fatalf("release: %v", err)
	}

	start := time.Now()
	ok, err := repo.Claim(ctx, "user-5", "pod-B", 30*time.Second)
	if err != nil {
		t.Fatalf("pod-B: %v", err)
	}
	if !ok {
		t.Fatal("pod-B could not take over after an explicit release; failover would be stuck behind the TTL")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("taking over after release took %s: expected it to be immediate", elapsed)
	}
}

// TestLease_SessionsAreIndependent: ownership is PER SESSION. A pod owning one
// session must not block another — that is what lets N replicas share load.
func TestLease_SessionsAreIndependent(t *testing.T) {
	repo := NewSessionLeaseRepository(openTestPostgres(t))
	ctx := context.Background()

	if ok, err := repo.Claim(ctx, "user-A", "pod-A", 30*time.Second); err != nil || !ok {
		t.Fatalf("pod-A/user-A: ok=%v err=%v", ok, err)
	}
	ok, err := repo.Claim(ctx, "user-B", "pod-B", 30*time.Second)
	if err != nil {
		t.Fatalf("pod-B/user-B: %v", err)
	}
	if !ok {
		t.Fatal("pod-B could not take a DIFFERENT session: ownership is blocking across sessions")
	}
}
