package domain

// CapabilityStatus classifies WHY a capability is or is not supported for a
// given (engine, account_type) pair. The values are deliberately NOT
// interchangeable synonyms for "no" — each names a different origin of
// failure, because the fix (and who owns it) differs by origin:
//
//   - not_implemented: wa-api never wrote the port/use case/route for this
//     capability, even though nothing blocks it. Fix: write it here.
//   - engine_unsupported: the underlying transport (noise socket or the
//     headless SPA) has no way to perform this operation at all. Fix:
//     implement it in that transport, or accept the gap (see
//     docs/CAPACIDADES.md "o que falta no próprio protocolo").
//   - upstream_unsupported: WhatsApp itself does not offer this operation
//     through the protocol this project speaks (WhatsApp Web multi-device),
//     regardless of transport. Fix: none — this is a ceiling, not a bug.
//   - account_type_unsupported: WhatsApp restricts the operation to a
//     different account type (e.g. catalog/products need a WABA-linked
//     business account that neither engine here reaches). Fix: none within
//     this project's engines.
//   - account_state_unsupported: the account could perform this in general,
//     but a per-account state blocks it right now (e.g. not verified, rate
//     limited). This is a RUNTIME precondition, not a static engine×type
//     fact — see CapabilityDecision.FailedPreconditions.
//   - permission_required: the capability needs a role/permission the
//     session does not currently hold (e.g. group admin).
//   - broken: the capability is wired end-to-end but is measured (in
//     HOUSEKEEP.md/field verification) to misbehave.
//   - partially_supported: some but not all of the capability's variants
//     work (e.g. a subset of media kinds).
//   - unknown: nobody has looked. This is the HONEST default until someone
//     verifies — never conflate with "not_implemented," which asserts a
//     conclusion the unknown case has not earned.
//   - supported: verified to work as documented.
type CapabilityStatus string

const (
	StatusSupported               CapabilityStatus = "supported"
	StatusPartiallySupported      CapabilityStatus = "partially_supported"
	StatusNotImplemented          CapabilityStatus = "not_implemented"
	StatusEngineUnsupported       CapabilityStatus = "engine_unsupported"
	StatusAccountTypeUnsupported  CapabilityStatus = "account_type_unsupported"
	StatusAccountStateUnsupported CapabilityStatus = "account_state_unsupported"
	StatusPermissionRequired      CapabilityStatus = "permission_required"
	StatusUpstreamUnsupported     CapabilityStatus = "upstream_unsupported"
	StatusBroken                  CapabilityStatus = "broken"
	StatusUnknown                 CapabilityStatus = "unknown"
)

// String makes CapabilityStatus printable without a conversion at every call
// site.
func (s CapabilityStatus) String() string { return string(s) }

// IsSupported reports whether this status should be read as a "yes" for
// CapabilityDecision.Supported. Only StatusSupported qualifies:
// partially_supported is deliberately NOT true here, because a caller
// checking "can I do this" wants a hard yes/no, and papering over a partial
// gap as "supported" is exactly the kind of silent optimism this registry
// exists to prevent.
func (s CapabilityStatus) IsSupported() bool {
	return s == StatusSupported
}

// EvidenceStatus separates HOW CONFIDENT the registry is in a support_status
// from the support_status itself (item 72 of the architectural prompt). Two
// cells can share the same CapabilityStatus while disagreeing sharply on this
// axis — e.g. "supported, confirmed" (read in HOUSEKEEP.md against a real
// account) versus "supported, probable" (the adapter file exists and has real
// logic, but nobody has photographed a device receiving it).
type EvidenceStatus string

const (
	// EvidenceConfirmed means a specific, citable measurement exists (a
	// HOUSEKEEP.md/ARMADILHAS.md entry, a field verification, a passing
	// integration test against a real account).
	EvidenceConfirmed EvidenceStatus = "confirmed"

	// EvidenceProbable means the adapter file was read and contains real
	// logic (not a stub returning a canned value) that plausibly implements
	// the capability, but nobody has verified the behavior against a live
	// account or device.
	EvidenceProbable EvidenceStatus = "probable"

	// EvidenceNotTested means a deliberate simplification: the dimension
	// (typically account_type) has never been exercised because nothing in
	// this codebase differentiates it yet, but the engine dimension for the
	// same capability was checked.
	EvidenceNotTested EvidenceStatus = "not_tested"

	// EvidenceUnknown is the honest default: nobody has looked at all.
	EvidenceUnknown EvidenceStatus = "unknown"
)

// String makes EvidenceStatus printable without a conversion at every call
// site.
func (e EvidenceStatus) String() string { return string(e) }
