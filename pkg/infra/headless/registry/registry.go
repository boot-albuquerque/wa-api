package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"wa-api/internal/headless"
)

// DefaultMaxSessions is the ceiling this process puts on concurrent headless
// sessions, and the number came from measurement rather than taste (decision
// 76, recorded in internal/headless/ESTADO.md).
//
// A browser costs ~650 MB of summed RSS with the SPA loaded, and the cost per
// browser FALLS as N grows (~620 MB at one, ~490 MB at eight) because RSS
// counts shared pages once per process. Four is therefore roughly 2.6 GB of
// upper-bound accounting, which is a ceiling a modest host survives while
// leaving room for the API itself.
//
// It is a DEFAULT, not a law: the right number depends on the host, and the
// only honest way to raise it is to measure that host.
const DefaultMaxSessions = 4

// DefaultMaxPairing is the SEPARATE quota for sessions still waiting to be
// paired, and it exists because of the holder inventory below (decision 77).
//
// It is deliberately smaller than the operational pool: pairing is rare and
// human, operating is continuous. A pairing quota the size of the operational
// pool would protect nothing — it would only move the starvation from one side
// to the other.
const DefaultMaxPairing = 2

// DefaultPairingDeadline is how long a session may wait on a human before it
// loses its slot.
//
// The number is not a promise about people: it is the point past which waiting
// stops meaning "someone is reaching for their phone" and starts meaning
// "nobody came". A WhatsApp QR code expires well before this, so a session past
// it is holding a slot for a code that no longer works.
const DefaultPairingDeadline = 5 * time.Minute

// ErrAtCapacity is returned when a NEW session would exceed the ceiling.
//
// It is an error and not a wait, and that distinction is the whole design.
// Blocking the caller until a slot frees is what F86 did, and the caller here
// is an HTTP handler: making it wait converts "this host is full" into "this
// request hangs", which is strictly worse and much harder to see.
var ErrAtCapacity = errors.New("headless: at session capacity")

// ErrUnknownSession is returned for a txtID this process does not hold.
var ErrUnknownSession = errors.New("headless: session not held by this process")

// ErrPairingAtCapacity is returned when a NEW pairing would exceed the pairing
// quota. It is deliberately distinct from ErrAtCapacity: the two mean different
// things to an operator — one says the host is busy serving, the other says too
// many people are mid-pairing — and one error for both would hide which.
var ErrPairingAtCapacity = errors.New("headless: at pairing capacity")

// Kind says which quota a session is admitted against.
//
// It is EXPLICIT at Acquire and never inferred, and that is not fussiness. If
// every new session entered through the pairing quota, restarting the process
// with N already-paired sessions would restore them all through a deliberately
// small quota — the protection would become the outage. That is regra 2 of
// CLAUDE.md: the input that turns this guard into the problem is the restart,
// so it is the input the design has to answer for.
type Kind int

const (
	// KindOperational is a session restored from an already-paired profile.
	KindOperational Kind = iota
	// KindPairing is a session showing a QR code, waiting for a human.
	KindPairing
)

func (k Kind) String() string {
	if k == KindPairing {
		return "pairing"
	}
	return "operational"
}

// Registry keeps track of which headless sessions this process holds.
//
// # The inventory of slot holders, which the ceiling obliges (CLAUDE.md, regra 1)
//
// Converting an unlimited resource into a limited one obliges naming everything
// that can occupy a slot and the WORST CASE of each. For this pool:
//
//	a session mounting the SPA        bounded by the settle budget
//	a session running an operation    bounded by the runner's deadline policy
//	a session whose browser died      bounded by the liveness probe plus grace
//	a session WAITING TO BE PAIRED    UNBOUNDED — it waits for a human with a phone
//
// The last one is the dangerous one, and it is dangerous by the project's own
// invariant: nothing that waits on a clock or a dead peer may occupy a limited
// slot. A session showing a QR code waits on something worse than a clock — a
// person. Four unpaired sessions would hold the whole pool indefinitely and
// starve every paired one.
//
// Decision 77 answers it: pairing sits OUTSIDE the operational pool, in a quota
// of its own with an explicit deadline. Human waiting never occupies a
// paired-session slot, which is the invariant restated in the only way that
// binds — two counters instead of one.
//
// Two things about that answer are load-bearing, and both are tested:
//
// The Kind is EXPLICIT at Acquire, never inferred. If every new session entered
// through the pairing quota, restarting with N already-paired sessions would
// restore them all through a deliberately small quota, and the guard would
// become the outage. That is the scenario where this mechanism CHARGES its
// price (regra 2), so it is the one the design answers for.
//
// Expired REPORTS rather than acts. Stopping a browser speaks CDP and takes
// seconds; doing it under this lock would make every other session's Acquire
// wait on an unrelated shutdown — the same reason Release stops outside the
// lock. Who stops the browser decides when.
type Registry struct {
	mu       sync.Mutex
	max      int
	maxPair  int
	deadline time.Duration
	// now is injected so the deadline test measures the RULE and not the
	// machine's clock. A test that sleeps to prove an expiry proves only that
	// sleeping works.
	now     func() time.Time
	holders map[string]*entry
}

// entry is one held session plus the two things the quotas need to know about
// it: which quota it counts against, and when it started waiting.
type entry struct {
	holder    *headless.Holder
	kind      Kind
	startedAt time.Time
}

// New builds a Registry with the given ceiling. Zero or negative uses
// DefaultMaxSessions rather than meaning "unlimited": an accidental zero would
// silently remove the protection, which is the failure mode a default exists to
// prevent.
func New(max int) *Registry {
	return NewWithQuotas(max, DefaultMaxPairing, DefaultPairingDeadline, time.Now)
}

// NewWithQuotas is New with the pairing quota, the pairing deadline and the
// clock spelled out. Zero or negative falls back to the default for each, for
// the same reason New does: an accidental zero would silently remove the
// protection.
func NewWithQuotas(max, maxPairing int, deadline time.Duration, now func() time.Time) *Registry {
	if max <= 0 {
		max = DefaultMaxSessions
	}
	if maxPairing <= 0 {
		maxPairing = DefaultMaxPairing
	}
	if deadline <= 0 {
		deadline = DefaultPairingDeadline
	}
	if now == nil {
		now = time.Now
	}
	return &Registry{
		max: max, maxPair: maxPairing, deadline: deadline, now: now,
		holders: make(map[string]*entry),
	}
}

// countLocked is how many entries count against each quota. The caller holds mu.
func (r *Registry) countLocked(k Kind) int {
	n := 0
	for _, e := range r.holders {
		if e.kind == k {
			n++
		}
	}
	return n
}

// Acquire returns the holder for txtID, creating it if this process does not
// have one yet.
//
// It NEVER blocks. An existing session is returned regardless of the ceiling —
// the ceiling bounds how many sessions exist, not how many times an existing one
// may be used, and refusing a session this process already holds would break
// working traffic to protect against a cost already paid.
func (r *Registry) Acquire(txtID string, cfg headless.StartConfig, kind Kind) (*headless.Holder, error) {
	if txtID == "" {
		return nil, fmt.Errorf("headless: empty txtID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if e, ok := r.holders[txtID]; ok {
		return e.holder, nil
	}
	if kind == KindPairing {
		if n := r.countLocked(KindPairing); n >= r.maxPair {
			return nil, fmt.Errorf("%w: %d of %d pairings in flight", ErrPairingAtCapacity, n, r.maxPair)
		}
	} else if n := r.countLocked(KindOperational); n >= r.max {
		return nil, fmt.Errorf("%w: %d of %d sessions held", ErrAtCapacity, n, r.max)
	}
	e := &entry{holder: headless.NewHolder(cfg), kind: kind, startedAt: r.now()}
	r.holders[txtID] = e
	return e.holder, nil
}

// Promote moves a pairing session into the operational pool, which is what a
// successful pairing means for capacity.
//
// The registry does NOT detect pairing itself: it does not talk to the page,
// and a component that guesses at SPA state would be a second, divergent
// answer to a question the page already answers. Whoever observes the pairing
// succeed calls this.
//
// A full operational pool makes this FAIL, and the session stays in the pairing
// quota rather than vanishing. A promotion that quietly dropped the entry would
// leak a live browser with no slot accounting for it.
func (r *Registry) Promote(txtID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.holders[txtID]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownSession, txtID)
	}
	if e.kind == KindOperational {
		return nil
	}
	if n := r.countLocked(KindOperational); n >= r.max {
		return fmt.Errorf("%w: %d of %d sessions held, %q stays in the pairing quota",
			ErrAtCapacity, n, r.max, txtID)
	}
	e.kind = KindOperational
	return nil
}

// Expired lists the pairing sessions past the deadline. It reports rather than
// acts: stopping a browser takes seconds and talks over CDP, and doing that
// under the registry lock would make every other session's Acquire wait on an
// unrelated shutdown — the same reason Release stops outside the lock.
func (r *Registry) Expired() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := r.now().Add(-r.deadline)
	var out []string
	for id, e := range r.holders {
		if e.kind == KindPairing && e.startedAt.Before(cutoff) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Holds answers the first half of the registry's job: does this process own
// this session?
//
// It is deliberately NOT "is it up". ADR-0005 D6 makes those two questions, and
// a readiness probe that conflates them lies — intent is not observed state.
func (r *Registry) Holds(txtID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.holders[txtID]
	return ok
}

// LenKind is how many sessions count against one quota.
func (r *Registry) LenKind(k Kind) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.countLocked(k)
}

// Len is how many sessions are held, which is what the ceiling counts.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.holders)
}

// Release stops the session and frees its slot.
//
// The slot is freed even when the stop was dirty. A holder that failed to go
// down cleanly is still gone as far as this process is concerned, and keeping
// its slot would leak capacity on exactly the failures that need capacity most.
func (r *Registry) Release(ctx context.Context, txtID string) (headless.StopVia, error) {
	r.mu.Lock()
	e, ok := r.holders[txtID]
	if ok {
		delete(r.holders, txtID)
	}
	r.mu.Unlock()

	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownSession, txtID)
	}
	h := e.holder
	// Stopping OUTSIDE the lock: a stop talks to a browser over CDP and can
	// take seconds, and holding the registry lock across it would make every
	// other session's Acquire wait on an unrelated shutdown.
	return h.Stop(ctx), nil
}
