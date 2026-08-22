package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"

	waheadless "wa-api/internal/wa-headless"
)

// DefaultMaxSessions is the ceiling this process puts on concurrent headless
// sessions, and the number came from measurement rather than taste (decision
// 76, recorded in internal/wa-headless/ESTADO.md).
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

// ErrAtCapacity is returned when a NEW session would exceed the ceiling.
//
// It is an error and not a wait, and that distinction is the whole design.
// Blocking the caller until a slot frees is what F86 did, and the caller here
// is an HTTP handler: making it wait converts "this host is full" into "this
// request hangs", which is strictly worse and much harder to see.
var ErrAtCapacity = errors.New("waheadless: at session capacity")

// ErrUnknownSession is returned for a txtID this process does not hold.
var ErrUnknownSession = errors.New("waheadless: session not held by this process")

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
// This type does NOT solve that, and says so rather than pretending: it caps
// what it can see, and the pairing case needs the SPA state, which means asking
// the page. Until that lands, a deployment that pairs more than the ceiling
// allows will starve, and that is a known limit rather than a surprise.
type Registry struct {
	mu      sync.Mutex
	max     int
	holders map[string]*waheadless.Holder
}

// New builds a Registry with the given ceiling. Zero or negative uses
// DefaultMaxSessions rather than meaning "unlimited": an accidental zero would
// silently remove the protection, which is the failure mode a default exists to
// prevent.
func New(max int) *Registry {
	if max <= 0 {
		max = DefaultMaxSessions
	}
	return &Registry{max: max, holders: make(map[string]*waheadless.Holder)}
}

// Acquire returns the holder for txtID, creating it if this process does not
// have one yet.
//
// It NEVER blocks. An existing session is returned regardless of the ceiling —
// the ceiling bounds how many sessions exist, not how many times an existing one
// may be used, and refusing a session this process already holds would break
// working traffic to protect against a cost already paid.
func (r *Registry) Acquire(txtID string, cfg waheadless.StartConfig) (*waheadless.Holder, error) {
	if txtID == "" {
		return nil, fmt.Errorf("waheadless: empty txtID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if h, ok := r.holders[txtID]; ok {
		return h, nil
	}
	if len(r.holders) >= r.max {
		return nil, fmt.Errorf("%w: %d of %d sessions held", ErrAtCapacity, len(r.holders), r.max)
	}
	h := waheadless.NewHolder(cfg)
	r.holders[txtID] = h
	return h, nil
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
func (r *Registry) Release(ctx context.Context, txtID string) (waheadless.StopVia, error) {
	r.mu.Lock()
	h, ok := r.holders[txtID]
	if ok {
		delete(r.holders, txtID)
	}
	r.mu.Unlock()

	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownSession, txtID)
	}
	// Stopping OUTSIDE the lock: a stop talks to a browser over CDP and can
	// take seconds, and holding the registry lock across it would make every
	// other session's Acquire wait on an unrelated shutdown.
	return h.Stop(ctx), nil
}
