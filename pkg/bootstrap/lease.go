package bootstrap

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Session ownership manager (ADR-0005, D2).
//
// The repository (pkg/infra/db/session_lease.go) answers "do I hold the
// right?". This file decides what to do with that answer over time, and that
// is where the dangerous part lives.
//
// F89 measured that the loser of a `StreamReplaced` stays dead forever.
// Coordinating who CONNECTS is therefore not enough: whoever stops being the
// owner has to drop the session on its own, instead of waiting for WhatsApp to
// tear it down — because once WhatsApp does, there is no way back.

const (
	envLeaseTTL       = "WA_API_LEASE_TTL_SECONDS"
	envLeaseHeartbeat = "WA_API_LEASE_HEARTBEAT_SECONDS"

	// A 15s TTL with a 5s heartbeat leaves 3x headroom against GC pauses and
	// network hiccups. Worst-case failover is TTL plus reconnect, and F89
	// measured reconnect at 1.2s to 2.3s — so the TTL dominates, and it is the
	// knob to turn if 17s is too slow.
	defaultLeaseTTLSeconds       = 15
	defaultLeaseHeartbeatSeconds = 5

	// ownerIDUnknownHost is the fallback identity when the hostname cannot be
	// read. Losing the hostname must not stop the process from claiming
	// ownership; it only makes the owner harder to trace.
	ownerIDUnknownHost = "unknown-host"
)

// leaseStore is what the manager needs from the repository. It is an interface
// so tests can substitute it — and the double must imitate the REAL rule, which
// lives in pkg/infra/db/session_lease.go: Claim returns (false, nil) when
// another owner holds a valid lease, and an error only when the DATABASE fails.
type leaseStore interface {
	Claim(ctx context.Context, userID, ownerID string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, userID, ownerID string) error
}

// leaseManager keeps ownership of this process's sessions alive.
type leaseManager struct {
	store     leaseStore
	ownerID   string
	ttl       time.Duration
	heartbeat time.Duration

	// onOwnershipLost disconnects the session. It is the piece F89's
	// measurement added to the original design: coordinating connects is not
	// enough, the former owner must let go by itself.
	onOwnershipLost func(userID string)

	mu sync.Mutex
	// lastRenewal records WHEN each session was last successfully renewed.
	// This is not diagnostics: it is what lets the manager decide ownership
	// without being able to reach the database — see renewOne.
	lastRenewal map[string]time.Time

	now func() time.Time // injectable so tests do not depend on the wall clock
}

func newLeaseManager(store leaseStore, ownerID string, ttl, heartbeat time.Duration, onOwnershipLost func(string)) *leaseManager {
	return &leaseManager{
		store:           store,
		ownerID:         ownerID,
		ttl:             ttl,
		heartbeat:       heartbeat,
		onOwnershipLost: onOwnershipLost,
		lastRenewal:     map[string]time.Time{},
		now:             time.Now,
	}
}

// buildOwnerID produces an identifier that is stable and unique per process.
//
// On k8s the hostname is the pod name, which is what an operator looks for when
// asking "who holds this session?". The PID disambiguates two processes on the
// same host, which is exactly the misconfigured `single` mode case.
func buildOwnerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		// Not fatal: ownership still works, since the PID keeps the id unique.
		// But it is worth a warning — the owner id is exactly what an operator
		// greps for when asking who holds a session, and "unknown-host" turns
		// that lookup into a dead end.
		log.Warn().Err(err).Str("fallback", ownerIDUnknownHost).
			Msg("could not read hostname for the lease owner id; ownership tracing will be harder")
		host = ownerIDUnknownHost
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// Claim attempts to take ownership of a session. Only the winner may connect.
func (m *leaseManager) Claim(ctx context.Context, userID string) (bool, error) {
	owned, err := m.store.Claim(ctx, userID, m.ownerID, m.ttl)
	if err != nil {
		return false, err
	}
	if owned {
		m.mu.Lock()
		m.lastRenewal[userID] = m.now()
		m.mu.Unlock()
	}
	return owned, nil
}

// renewOne renews one session's lease and reports whether it is STILL ours.
//
// The error rule is the part that usually comes out wrong. A renewal can fail
// for two distinct reasons that demand opposite responses:
//
//   - Another owner took over (ok == false): we lost, and must let go now.
//   - The database did not answer (error): we do not know. Killing a healthy
//     session over a network blip is worse than the problem we are solving.
//
// But tolerating errors forever is unsafe: once the TTL has passed without a
// confirmed renewal, the lease has EXPIRED from every other replica's point of
// view, and any of them may legitimately take over. Continuing to serve at that
// point creates exactly the two simultaneous owners this mechanism prevents.
//
// So the decision is not "did the database answer?" but "how long since the
// last CONFIRMED renewal?".
func (m *leaseManager) renewOne(ctx context.Context, userID string) bool {
	owned, err := m.store.Claim(ctx, userID, m.ownerID, m.ttl)

	switch {
	case err == nil && owned:
		m.mu.Lock()
		m.lastRenewal[userID] = m.now()
		m.mu.Unlock()
		return true

	case err == nil && !owned:
		log.Warn().Str("userid", userID).Str("owner", m.ownerID).
			Msg("session ownership taken by another replica; releasing")
		return false

	default:
		m.mu.Lock()
		last := m.lastRenewal[userID]
		m.mu.Unlock()

		elapsed := m.now().Sub(last)
		if elapsed < m.ttl {
			log.Warn().Err(err).Str("userid", userID).Dur("since_last_renewal", elapsed).
				Msg("lease renewal failed; still within TTL, keeping the session")
			return true
		}
		log.Error().Err(err).Str("userid", userID).Dur("since_last_renewal", elapsed).
			Dur("ttl", m.ttl).
			Msg("lease expired without a confirmed renewal; releasing to avoid two owners")
		return false
	}
}

// RunHeartbeat renews, in a loop, every lease this process holds.
//
// It returns when the context is done. It deliberately does NOT release leases
// on the way out: graceful shutdown calls ReleaseAll explicitly, and a
// cancellation caused by a crash should not try to write to a database that is
// probably the cause of the crash.
func (m *leaseManager) RunHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(m.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, userID := range m.ownedSessions() {
				if m.renewOne(ctx, userID) {
					continue
				}
				m.forget(userID)
				if m.onOwnershipLost != nil {
					m.onOwnershipLost(userID)
				}
			}
		}
	}
}

func (m *leaseManager) ownedSessions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.lastRenewal))
	for id := range m.lastRenewal {
		ids = append(ids, id)
	}
	return ids
}

func (m *leaseManager) forget(userID string) {
	m.mu.Lock()
	delete(m.lastRenewal, userID)
	m.mu.Unlock()
}

// Release hands back a single lease.
//
// Used when a session was claimed but never came up: the claim happens BEFORE
// the session is materialized (so a denial leaves no dirty state), which means
// a failure afterwards would otherwise keep the lease alive forever — the
// heartbeat renewing ownership of a session that does not exist, and no other
// replica ever able to take that user (F96, measured).
func (m *leaseManager) Release(ctx context.Context, userID string) {
	if err := m.store.Release(ctx, userID, m.ownerID); err != nil {
		// Not fatal: the TTL expires this lease on its own. Worth a warning
		// because until then the session is unreachable to every replica.
		log.Warn().Err(err).Str("userid", userID).
			Msg("failed to release lease for a session that did not start; the TTL will clear it")
	}
	m.forget(userID)
}

// ReleaseAll drops every lease on graceful shutdown so failover does not wait
// out the TTL. A failure here is logged and does not stop the loop: the process
// is exiting anyway, and the TTL covers whatever is left behind.
func (m *leaseManager) ReleaseAll(ctx context.Context) {
	for _, userID := range m.ownedSessions() {
		if err := m.store.Release(ctx, userID, m.ownerID); err != nil {
			log.Warn().Err(err).Str("userid", userID).
				Msg("failed to release lease during shutdown; the TTL covers it")
			continue
		}
		m.forget(userID)
	}
}

// leaseSettings reads the TTL and heartbeat from the environment and rejects
// invalid combinations.
//
// The heartbeat must fit inside the TTL with room to spare: with an interval
// greater than or equal to the TTL, the very first renewal already arrives late
// and the session would be dropped during NORMAL operation. Requiring at most
// half leaves room for one lost renewal without losing ownership.
func leaseSettings() (ttl, heartbeat time.Duration, err error) {
	ttlSeconds := readIntFromEnv(envLeaseTTL, defaultLeaseTTLSeconds)
	heartbeatSeconds := readIntFromEnv(envLeaseHeartbeat, defaultLeaseHeartbeatSeconds)

	if ttlSeconds <= 0 || heartbeatSeconds <= 0 {
		return 0, 0, fmt.Errorf("%s=%d and %s=%d: both must be positive",
			envLeaseTTL, ttlSeconds, envLeaseHeartbeat, heartbeatSeconds)
	}
	if heartbeatSeconds*2 > ttlSeconds {
		return 0, 0, fmt.Errorf(
			"%s=%ds must be at most HALF of %s=%ds: with a larger interval a single lost renewal already costs ownership",
			envLeaseHeartbeat, heartbeatSeconds, envLeaseTTL, ttlSeconds)
	}
	return time.Duration(ttlSeconds) * time.Second, time.Duration(heartbeatSeconds) * time.Second, nil
}
