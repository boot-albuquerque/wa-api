// Package capabilityregistry resolves, per (capability, engine, account_type),
// WHETHER an operation is supported and WHY — without executing it.
//
// # What this package is not
//
// It is not a dispatcher. There is deliberately no
// `Execute(capability, map[string]any)` method anywhere here: doing so would
// let a single loosely-typed entry point stand in for the ~40 typed
// application ports in pkg/application/contracts, destroying the compile-time
// safety ADR-001 and the port-splitting decisions (82/87/92) built up. Actual
// execution keeps going through those typed ports; this package only answers
// "should I even try, and if not, why."
//
// It is also not a place for business rules. A CapabilityProvider answers
// from a STATIC table (pkg/capabilityregistry/matrix.go) built by reading
// adapter code, not by re-deriving policy at request time. If a rule belongs
// here it is "engine X does/doesn't serve capability Y for account type Z" —
// nothing else.
package capabilityregistry

import "wa-api/pkg/domain"

// CapabilityDecision is the answer to "can this session, on this engine and
// account type, perform this capability, and why."
type CapabilityDecision struct {
	// Capability is the business operation being asked about.
	Capability domain.Capability

	// Supported is the hard yes/no a caller checks before calling the typed
	// port. It is true only when Status == domain.StatusSupported — see
	// domain.CapabilityStatus.IsSupported for why partially_supported does
	// not count.
	Supported bool

	// Status explains what kind of "no" this is, when it is one. See
	// domain.CapabilityStatus for the full taxonomy and why the values are
	// not interchangeable.
	Status domain.CapabilityStatus

	// Reason is a short, human-readable sentence a log line or an API error
	// body can show as-is. It restates Status with the specific engine/
	// account_type/capability filled in — never a stand-in for Status
	// itself (callers must branch on Status, not parse Reason).
	Reason string

	// Engine and AccountType echo the inputs the decision was made for, so a
	// decision can be logged or serialized without also carrying the
	// request that produced it.
	Engine      domain.Engine
	AccountType domain.AccountType

	// RequiredPreconditions lists what has to hold, in general, for this
	// capability to work (e.g. "group admin", "verified business account").
	// It is STATIC — the same list regardless of whether the current
	// session meets it.
	RequiredPreconditions []string

	// FailedPreconditions lists which of RequiredPreconditions this
	// evaluation could not confirm. The registry never fills this in on its
	// own — it has no access to session state — a caller that has already
	// checked runtime preconditions (permission, account state) passes them
	// in via CapabilityRegistry.Decide so the decision reflects them; an
	// empty slice here means "unevaluated," not "all satisfied."
	FailedPreconditions []string

	// Evidence separates HOW CONFIDENT this decision is from Status itself.
	// See domain.EvidenceStatus.
	Evidence domain.EvidenceStatus
}
