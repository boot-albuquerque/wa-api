package waheadless

import (
	"context"
	"fmt"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

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
	configFor func(txtID string) (waheadless.StartConfig, error)
	runner    *waheadless.Runner
}

// NewSessions builds the seam.
func NewSessions(reg *registry.Registry, configFor func(string) (waheadless.StartConfig, error)) *Sessions {
	return &Sessions{registry: reg, configFor: configFor, runner: waheadless.NewRunner()}
}

// Runner is the deadline policy and operation log every capability needs.
func (s *Sessions) Runner() *waheadless.Runner { return s.runner }

// Holds reports whether this process owns the session, WITHOUT booting it.
//
// ADR-0005 D6 makes ownership and readiness two questions; a guard that booted
// a browser to answer the first would turn a cheap check into a minute of work.
func (s *Sessions) Holds(txtID string) bool { return s.registry.Holds(txtID) }

// EnsureSession is the SessionGuard half every transport port embeds.
func (s *Sessions) EnsureSession(_ context.Context, txtID string) error {
	if !s.registry.Holds(txtID) {
		return fmt.Errorf("%w: %q", registry.ErrUnknownSession, txtID)
	}
	return nil
}

// Evaluator resolves txtID into a way to reach its page.
//
// The order is load-bearing and tested: the config is resolved BEFORE a slot is
// acquired, so a session nobody configured cannot consume capacity that a
// working session needs.
func (s *Sessions) Evaluator(ctx context.Context, txtID string) (waheadless.Evaluator, error) {
	cfg, err := s.configFor(txtID)
	if err != nil {
		return nil, fmt.Errorf("waheadless: config for session: %w", err)
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
