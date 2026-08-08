package bootstrap

import (
	"context"
	"fmt"
	"testing"
	"time"

	"wa-api/pkg/infra/db"
)

// ADR-0005 D2, wiring. These tests pin the decisions that connect the manager
// to the process — the ones a reader would otherwise have to infer from call
// sites scattered across main.go and lifecycle.go.

// TestBuildLeaseManager_NilInSingleMode: in `single` there is nothing to
// coordinate, and D1 already guarantees exclusivity with an OS flock. Building
// a manager there would add periodic database writes and buy nothing.
//
// The nil is load-bearing, not an omission: every call site treats nil as
// "ownership disabled".
func TestBuildLeaseManager_NilInSingleMode(t *testing.T) {
	t.Setenv(envClusterMode, clusterModeSingle)

	manager, err := buildLeaseManager(&server{})
	if err != nil {
		t.Fatalf("single mode rejected: %v", err)
	}
	if manager != nil {
		t.Error("built a lease manager in single mode; it would write to the database for nothing")
	}
}

// TestBuildLeaseManager_RejectsBadSettings: an impossible TTL/heartbeat pair
// must stop startup, not produce a manager that drops sessions during normal
// operation.
func TestBuildLeaseManager_RejectsBadSettings(t *testing.T) {
	t.Setenv(envClusterMode, clusterModeMulti)
	t.Setenv(envLeaseTTL, "5")
	t.Setenv(envLeaseHeartbeat, "5") // heartbeat must be at most half the TTL

	if _, err := buildLeaseManager(&server{}); err == nil {
		t.Error("accepted a heartbeat equal to the TTL; sessions would be dropped in normal operation")
	}
}

// TestClaimSessionOwnership_NilManagerAlwaysAllows: single mode must not be
// held back by a mechanism it does not use.
func TestClaimSessionOwnership_NilManagerAlwaysAllows(t *testing.T) {
	if !claimSessionOwnership(nil, "u1") {
		t.Error("blocked a session with no manager; single mode would never connect anything")
	}
}

// TestClaimSessionOwnership_DatabaseFailureDenies pins the conservative
// direction, which is the opposite of what "keep working no matter what"
// intuition suggests.
//
// Refusing to connect leaves the session unserved for one TTL. Connecting
// without confirmed ownership risks two live owners — and F89 measured that
// the loser of that race dies permanently, with no reconnect. Unserved for a
// few seconds is recoverable; permanently dead is not.
func TestClaimSessionOwnership_DatabaseFailureDenies(t *testing.T) {
	store := newFakeLeaseStore()
	store.failWith(fmt.Errorf("%w: connection refused", db.ErrLeaseUnavailable))
	manager := newLeaseManager(store, "pod-A", 15*time.Second, 5*time.Second, nil)

	if claimSessionOwnership(manager, "u1") {
		t.Error("connected without confirmed ownership; two live owners is worse than one unserved session")
	}
}

// TestClaimSessionOwnership_DeniedWhenAnotherOwnerHolds: the plain case, and
// the reason the filter exists.
func TestClaimSessionOwnership_DeniedWhenAnotherOwnerHolds(t *testing.T) {
	store := newFakeLeaseStore()
	store.giveTo("u1", "pod-B")
	manager := newLeaseManager(store, "pod-A", 15*time.Second, 5*time.Second, nil)

	if claimSessionOwnership(manager, "u1") {
		t.Error("claimed a session already owned by another replica")
	}
}

// TestReleaseLeasesOnShutdown_NilManagerIsSafe: shutdown runs in every mode,
// and single mode has no manager. A panic here would turn a clean stop into a
// crash.
func TestReleaseLeasesOnShutdown_NilManagerIsSafe(t *testing.T) {
	releaseLeasesOnShutdown(nil) // must not panic
}

// TestReleaseLeasesOnShutdown_HandsLeasesBack: without this, failover waits out
// the full TTL on every deploy.
func TestReleaseLeasesOnShutdown_HandsLeasesBack(t *testing.T) {
	store := newFakeLeaseStore()
	manager := newLeaseManager(store, "pod-A", 15*time.Second, 5*time.Second, nil)

	for _, userID := range []string{"u1", "u2"} {
		if ok, err := manager.Claim(context.Background(), userID); err != nil || !ok {
			t.Fatalf("claim %s: ok=%v err=%v", userID, ok, err)
		}
	}

	releaseLeasesOnShutdown(manager)

	if left := store.remaining(); left != 0 {
		t.Errorf("%d leases left behind; the next replica would wait out the TTL", left)
	}
}

// TestStartLeaseHeartbeat_NilManagerIsSafe: same reasoning as shutdown — the
// call site is unconditional, the manager is not.
func TestStartLeaseHeartbeat_NilManagerIsSafe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startLeaseHeartbeat(ctx, nil) // must not panic
}

// TestLeaseReleaseTimeout_IsSeparateFromShutdownBudget documents a constraint
// that is invisible at the call site.
//
// The shutdown path gives srv.Shutdown a 5s context. Lease release deliberately
// uses its own, shorter budget: sharing one would make the two compete, and
// releasing leases would lose to HTTP connections draining — silently turning
// every deploy into a full-TTL failover.
//
// If someone later "simplifies" this by reusing the shutdown context, this test
// is the note explaining why not.
func TestLeaseReleaseTimeout_IsSeparateFromShutdownBudget(t *testing.T) {
	const shutdownBudget = 5 * time.Second
	if leaseReleaseTimeout >= shutdownBudget {
		t.Errorf("leaseReleaseTimeout (%s) must stay below the shutdown budget (%s), or lease release competes with connection draining",
			leaseReleaseTimeout, shutdownBudget)
	}
}
