package capabilityregistry

import "wa-api/pkg/domain"

// CapabilityProvider answers what one ENGINE can do, in general — before any
// particular session or account type is known.
//
// It is intentionally enxuta (three methods): Engine identifies which
// transport this is, Capabilities enumerates what the matrix has an entry
// for at all (so a caller can list "what could this engine ever do" without
// probing every known capability one by one), and Supports resolves a single
// (capability, account_type) pair. Execution is NOT here — see the package
// doc comment for why.
type CapabilityProvider interface {
	// Engine identifies the transport this provider answers for.
	Engine() domain.Engine

	// Capabilities lists every capability the matrix has at least one entry
	// for, on this engine, across any account type. It is the enumeration a
	// caller uses to build a menu of "what might work here" — not a claim
	// that all of them are supported.
	Capabilities() []domain.Capability

	// Supports resolves one (capability, accountType) pair against the
	// static matrix for this provider's engine. It never consults session
	// state — see CapabilityRegistry.Decide for the entry point that also
	// folds in runtime preconditions.
	Supports(capability domain.Capability, accountType domain.AccountType) CapabilityDecision
}

// matrixProvider is the one CapabilityProvider implementation: a thin,
// engine-scoped view over the shared matrix. There is no reason for a second
// implementation to diverge in behavior — both engines are looked up the
// same way — so a single generic type is used instead of one struct per
// engine.
type matrixProvider struct {
	engine domain.Engine
	m      *matrix
}

var _ CapabilityProvider = (*matrixProvider)(nil)

func (p *matrixProvider) Engine() domain.Engine { return p.engine }

func (p *matrixProvider) Capabilities() []domain.Capability {
	return p.m.capabilitiesForEngine(p.engine)
}

func (p *matrixProvider) Supports(capability domain.Capability, accountType domain.AccountType) CapabilityDecision {
	return p.m.decide(capability, p.engine, accountType)
}
