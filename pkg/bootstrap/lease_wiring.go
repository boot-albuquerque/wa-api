package bootstrap

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"wa-api/pkg/infra/db"
)

// Wiring for session ownership (ADR-0005, D2).
//
// lease.go holds the policy; this file connects it to the process: who builds
// the manager, what happens when ownership is lost, and how long shutdown may
// spend giving leases back.

// leaseReleaseTimeout bounds how long shutdown waits to hand leases back.
//
// It is deliberately NOT the same context as srv.Shutdown: sharing one budget
// makes the two compete, and lease release would lose to HTTP connections
// draining. Short on purpose — if the database is slow right now, the TTL
// already covers whatever we fail to release.
const leaseReleaseTimeout = 3 * time.Second

// buildLeaseManager returns the ownership manager, or nil when this process
// does not need one.
//
// It returns nil in `single` mode BY DESIGN. There is no ownership to
// coordinate with a single process, and D1 already guarantees exclusivity with
// an OS-level flock on the data directory. Running the lease there would add
// periodic writes to the database and buy nothing the flock does not already
// provide.
func buildLeaseManager(s *server) (*leaseManager, error) {
	mode, err := clusterModeFromEnv()
	if err != nil {
		return nil, err
	}
	if mode != clusterModeMulti {
		return nil, nil
	}

	ttl, heartbeat, err := leaseSettings()
	if err != nil {
		return nil, err
	}

	store := db.NewSessionLeaseRepository(s.DB)
	ownerID := buildOwnerID()
	log.Info().
		Str("owner_id", ownerID).
		Dur("ttl", ttl).
		Dur("heartbeat", heartbeat).
		Msg("session ownership enabled")

	return newLeaseManager(store, ownerID, ttl, heartbeat, releaseSessionLocally), nil
}

// setupSessionOwnership builds the manager and installs it on the server, or
// stops the process if ownership cannot be configured.
//
// It exists as its own function, rather than inline in Main, because Main is
// the most complex function in the repository and the lint gate tracks exactly
// that: the ceiling is the WORST function, not the total count. Moving a branch
// out of Main is the improvement the gate asks for — decomposing raises the
// issue count and lowers the ceiling.
//
// A failure here is fatal on purpose. An unusable ownership configuration in
// `multi` means the process cannot know which sessions are its own, and
// connecting without that knowledge is how two live owners are created.
func setupSessionOwnership(s *server) {
	manager, err := buildLeaseManager(s)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid session ownership configuration")
	}
	s.Leases = manager
}

// releaseSessionLocally drops a session this process no longer owns.
//
// It tears down the transport and the in-memory registries, and DELIBERATELY
// does not touch `users.connected`.
//
// That column is SHARED state: the new owner needs it to stay 1 so its own
// connectOnStartup picks the session up. The kill-channel path
// (session_attach_hook_adapter.go) does write connected=0, which is right when
// the USER asked to stop — and wrong here, where the session must keep running,
// just somewhere else. Reusing that path would turn "this pod lost the lease"
// into "this session is off for everyone".
func releaseSessionLocally(userID string) {
	log.Warn().Str("userid", userID).
		Msg("releasing session locally after losing ownership; the new owner takes it from here")

	if client := clientManager.GetWaNoiseClient(userID); client != nil {
		client.Disconnect()
	}
	clientManager.DeleteWaNoiseClient(userID)
	clientManager.DeleteUserClient(userID)
	clientManager.DeleteHTTPClient(userID)
}

// claimSessionOwnership reports whether this process may serve a session.
//
// With no manager (single mode) the answer is always yes: there is no one to
// compete with.
//
// A database failure answers NO. That is the conservative direction: refusing
// to connect leaves the session unserved for one TTL, while connecting without
// confirmed ownership risks two live owners — and F89 measured that the loser
// of that race dies permanently.
func claimSessionOwnership(manager *leaseManager, userID string) bool {
	if manager == nil {
		return true
	}
	owned, err := manager.Claim(context.Background(), userID)
	if err != nil {
		log.Error().Err(err).Str("userid", userID).
			Msg("could not confirm session ownership; skipping this session rather than risking two owners")
		return false
	}
	if !owned {
		log.Info().Str("userid", userID).
			Msg("session owned by another replica; skipping")
	}
	return owned
}

// releaseSessionOwnership hands a lease back when a session failed to start.
//
// No-op with no manager: `single` mode never claimed anything.
func releaseSessionOwnership(manager *leaseManager, userID string) {
	if manager == nil {
		return
	}
	log.Warn().Str("userid", userID).
		Msg("session did not start; handing its ownership back so another replica can take it")
	manager.Release(context.Background(), userID)
}

// startLeaseHeartbeat runs the renewal loop when there is a manager.
func startLeaseHeartbeat(ctx context.Context, manager *leaseManager) {
	if manager == nil {
		return
	}
	safeGo("lease-heartbeat", func() { manager.RunHeartbeat(ctx) })
}

// releaseLeasesOnShutdown hands every lease back so failover does not wait out
// the TTL.
//
// It MUST be called explicitly, never via defer: the shutdown path ends in
// os.Exit(0), and os.Exit does not run deferred functions. A deferred release
// would silently never run, and the symptom would be "sometimes a session takes
// 15s to come back" — blamed on the network, not on a defer that never fired.
func releaseLeasesOnShutdown(manager *leaseManager) {
	if manager == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), leaseReleaseTimeout)
	defer cancel()

	// Count before releasing, and report what is LEFT afterwards. A bare
	// "leases released" cannot be acted on: what an operator needs to know at
	// this point is whether any lease stayed behind, because each one costs
	// the next replica a full TTL before it can take the session over.
	held := len(manager.ownedSessions())
	manager.ReleaseAll(ctx)
	stillHeld := len(manager.ownedSessions())

	log.Info().
		Int("released", held-stillHeld).
		Int("still_held", stillHeld).
		Dur("budget", leaseReleaseTimeout).
		Msg("session leases handed back on shutdown")
}
