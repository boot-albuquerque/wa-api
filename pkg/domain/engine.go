package domain

import "fmt"

// Engine names the transport that serves a session.
//
// # Why this is a domain concept and not a configuration string
//
// Two transports serve the same ports: the socket (`wa_noise`) and the
// browser-driven SPA (`wa_headless`). They do NOT serve the same set of
// capabilities, so which one runs a session is an operational decision that
// has to be READ BACK — from the database, from the API, from a log line —
// and not re-derived from process configuration on every startup.
//
// # The temporary duality with pkg/bootstrap/engine_selection.go
//
// pkg/bootstrap declares EngineWaNoise = "wanoise" and EngineWaHeadless =
// "headless". Those are the INFRASTRUCTURE CONFIGURATION strings, read from
// WA_API_ENGINE and WA_API_ENGINE_HEADLESS_SESSIONS, and they predate this
// type. The values here are the PUBLIC CONTRACT values, in snake_case, and
// they are deliberately different text.
//
// The two are not unified yet on purpose: unifying them means changing what
// operators already have in their environment, and that belongs to the
// worktree that makes routing consume domain.Engine (capability-registry /
// routing). Until then EngineFromInfraName is the single crossing point
// between the two vocabularies. Registered in HOUSEKEEP.md.
type Engine string

const (
	// EngineWaNoise is the socket transport. It is the historical default.
	EngineWaNoise Engine = "wa_noise"

	// EngineWaHeadless is the browser-driven SPA transport.
	EngineWaHeadless Engine = "wa_headless"

	// EngineLegacyUnknown is INTERNAL ONLY: it marks a row whose engine was
	// never recorded because the column did not exist when the row was
	// written.
	//
	// It is never a valid value for creating a session — IsValidForCreate
	// rejects it — and it is never an expected steady state either: the
	// startup backfill replaces every occurrence of it. It exists so that a
	// row can say "we do not know" instead of silently claiming an engine it
	// may not be running, which is the failure mode this repository keeps
	// paying for elsewhere.
	EngineLegacyUnknown Engine = "legacy_unknown"
)

// ErrInvalidEngine is returned when a value is not an engine this contract
// accepts for creating a session.
var ErrInvalidEngine = fmt.Errorf("invalid engine: must be %q or %q",
	EngineWaNoise, EngineWaHeadless)

// String makes Engine printable without a conversion at every call site.
func (e Engine) String() string { return string(e) }

// IsValidForCreate reports whether this engine may be chosen when a session is
// created.
//
// legacy_unknown is deliberately REJECTED here: accepting it would let a
// caller write "we do not know" as an intent, and the whole point of the value
// is that it describes history, never a choice.
func (e Engine) IsValidForCreate() bool {
	switch e {
	case EngineWaNoise, EngineWaHeadless:
		return true
	default:
		return false
	}
}

// IsKnown reports whether this engine is any value this domain recognizes,
// legacy_unknown included.
//
// It is the check for READING a stored row, where legacy_unknown is a legal
// (if undesirable) state. Creation uses IsValidForCreate instead — keeping the
// two questions apart is what stops a read-path relaxation from quietly
// widening the write path.
func (e Engine) IsKnown() bool {
	return e.IsValidForCreate() || e == EngineLegacyUnknown
}

// ParseEngine converts external text into an Engine accepted for creation.
//
// It does not trim, lowercase, or otherwise repair its input: a value that
// needs repairing is a value whose author is not sure what they meant, and
// guessing here would put the guess in the database.
func ParseEngine(raw string) (Engine, error) {
	e := Engine(raw)
	if !e.IsValidForCreate() {
		return "", fmt.Errorf("%w (got %q)", ErrInvalidEngine, raw)
	}
	return e, nil
}
