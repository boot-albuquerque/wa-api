package headless

import (
	"context"
	"fmt"

	"wa-api/internal/headless"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/headless/registry"
)

// ErrNoSession is the typed error every port-boundary method on Sessions
// returns for a txtID this process does not hold — the SAME shape
// pkg/infra/noise/runtime/session.ErrNoSession already returns for the
// identical condition, kept as its own local constructor (not a shared
// import) because the two engine packages must not need to import each
// other for a four-line error constructor.
//
// # Why this exists (HOUSEKEEP F379)
//
// Until this fix, EnsureSession returned a bare
// fmt.Errorf("%w: %q", registry.ErrUnknownSession, txtID) — not an
// *apperr.AppError. RespondJSON (pkg/presentation/http/response.go) only
// derives the HTTP status from errors.As(err, *apperr.AppError); anything
// else falls back to the literal status the handler passed in, which every
// /session/* handler hardcodes to 500. MEASURED live: a headless session
// that never connected got 500 internal_error from GET /session/hmac/config
// and POST /session/history, while the IDENTICAL scenario on a noise
// session — same handler, same use case, only the engine differs — got the
// correct 400 no_session. This one error shape being untyped broke the
// error contract for every use case that goes through EnsureSession for
// headless: HOUSEKEEP F273/F281's own comment lists eleven of them
// (S3/HMAC/proxy/history config, ListUsers, DeleteUserComplete, and the
// three SessionController methods), not only pairing.
func ErrNoSession(txtID string, cause error) *apperr.AppError {
	if cause == nil {
		cause = fmt.Errorf("%w: %q", registry.ErrUnknownSession, txtID)
	}
	return apperr.New("no_session", apperr.CategoryValidation, "no session", false, cause)
}

// Sessions is the seam every adapter goes through to reach a page (decision 83).
//
// # Why it exists as its own type
//
// Each adapter used to carry its own copy of the same four steps — look up the
// config, acquire a slot, boot or reuse the session, hand back the evaluator.
// Six copies of code that no unit test could reach, because the third step
// needs a real browser, and six places for the first two steps to drift apart.
//
// Concentrating them here makes the two steps that DO have rules testable once
// instead of six times untestable:
//
//	config lookup failure   must propagate, not become an empty answer
//	capacity refusal        must reach the caller as an error, never as silence
//
// What remains untestable without a browser is exactly one line — booting the
// session — and that is covered by the integration suite, which is the only
// place that can honestly cover it.
//
// The Holder is deliberately NOT injectable: it owns the ownership invariant
// (one profile, one active session), and a fake that satisfied its interface
// would be a second, more permissive answer to that question — the trap this
// repository catalogues as number one.
type Sessions struct {
	registry  *registry.Registry
	configFor func(txtID string) (headless.StartConfig, error)
	runner    *headless.Runner
}

// NewSessions builds the seam.
func NewSessions(reg *registry.Registry, configFor func(string) (headless.StartConfig, error)) *Sessions {
	return &Sessions{registry: reg, configFor: configFor, runner: headless.NewRunner()}
}

// Runner is the deadline policy and operation log every capability needs.
func (s *Sessions) Runner() *headless.Runner { return s.runner }

// Holds reports whether this process owns the session, WITHOUT booting it.
//
// ADR-0005 D6 makes ownership and readiness two questions; a guard that booted
// a browser to answer the first would turn a cheap check into a minute of work.
func (s *Sessions) Holds(txtID string) bool { return s.registry.Holds(txtID) }

// EnsureSession is the SessionGuard half every transport port embeds.
//
// Returns ErrNoSession (an *apperr.AppError), not the bare registry
// sentinel — see ErrNoSession's own doc comment (HOUSEKEEP F379) for why
// that distinction is the whole fix.
func (s *Sessions) EnsureSession(_ context.Context, txtID string) error {
	if !s.registry.Holds(txtID) {
		return ErrNoSession(txtID, nil)
	}
	return nil
}

// Release stops the session and frees its slot, reporting HOW it went down.
//
// A dirty stop still frees the slot: a holder that failed to go down cleanly is
// gone as far as this process is concerned, and keeping its slot would leak
// capacity on exactly the failures that need capacity most.
//
// The error path is ONLY "txtID is not held" — registry.Registry.Release's
// own doc comment confirms the actual stop outcome travels in StopVia, never
// as an error — so wrapping it unconditionally in ErrNoSession here is safe:
// there is no OTHER failure this could be masking (HOUSEKEEP F379, same fix
// as EnsureSession above). Before this fix, GET /session/disconnect on a
// headless session nobody had connected answered 500 internal_error;
// the identical scenario on noise already answered 400 no_session.
func (s *Sessions) Release(ctx context.Context, txtID string) (headless.StopVia, error) {
	via, err := s.registry.Release(ctx, txtID)
	if err != nil {
		return via, ErrNoSession(txtID, err)
	}
	return via, nil
}

// Evaluator resolves txtID into a way to reach its page.
//
// The order is load-bearing and tested: the config is resolved BEFORE a slot is
// acquired, so a session nobody configured cannot consume capacity that a
// working session needs.
func (s *Sessions) Evaluator(ctx context.Context, txtID string) (headless.Evaluator, error) {
	cfg, err := s.configFor(txtID)
	if err != nil {
		return nil, fmt.Errorf("headless: config for session: %w", err)
	}
	holder, err := s.registry.Acquire(txtID, cfg, registry.KindOperational)
	if err != nil {
		return nil, err
	}
	// The one line no unit test can honestly reach: it starts a browser. The
	// integration suite covers it.
	sess, err := holder.Session(ctx)
	if err != nil {
		return nil, err
	}
	return sess.Tab().Evaluate, nil
}

// EvaluatorForPairing is Evaluator's counterpart for a session that has not
// paired yet: it acquires against the SEPARATE pairing quota
// (registry.KindPairing, registry.DefaultMaxPairing) instead of the
// operational one, so a burst of pairing attempts cannot starve sessions
// that are already up. Acquire returns the existing entry for an id already
// held, regardless of kind (registry.go), so calling this on an id already
// promoted to KindOperational still resolves to its (single) holder.
//
// It boots through Holder.PairingSession, not Holder.Session: the latter
// (core.StartSession) refuses any page that is not already APP_READY, by
// design (HOUSEKEEP H145) — exactly the QR/pairing screen this method
// exists to reach.
//
// Same load-bearing order as Evaluator: config before Acquire.
func (s *Sessions) EvaluatorForPairing(ctx context.Context, txtID string) (headless.Evaluator, error) {
	cfg, err := s.configFor(txtID)
	if err != nil {
		return nil, fmt.Errorf("headless: config for session: %w", err)
	}
	holder, err := s.registry.Acquire(txtID, cfg, registry.KindPairing)
	if err != nil {
		return nil, err
	}
	sess, err := holder.PairingSession(ctx)
	if err != nil {
		return nil, err
	}
	return sess.Tab().Evaluate, nil
}

// Promote moves txtID from the pairing quota to the operational one, once a
// caller has observed pairing succeed. See registry.Registry.Promote: the
// registry does not detect pairing itself, so whoever does (the QR/pairphone
// adapters, via owner.Refresh) must call this.
func (s *Sessions) Promote(txtID string) error {
	return s.registry.Promote(txtID)
}
