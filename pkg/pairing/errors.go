package pairing

import (
	"fmt"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// The four refusals of the pairing surface. Each is an apperr.AppError, so the
// HTTP boundary derives the status from the category
// (pkg/presentation/http/response.go ignores the status a call site passes when
// the error is an AppError) and no handler repeats a number.
const (
	// CodeInvalidEngine marks a pairing request whose `engine` is missing,
	// empty, or not one of the two the contract accepts.
	//
	// legacy_unknown is REJECTED here, and that is not an oversight:
	// domain.Engine.IsValidForCreate refuses it for the same reason
	// (pkg/domain/engine.go) — the value describes history, never an intent.
	CodeInvalidEngine = "invalid_engine"

	// CodeEngineMismatch marks a request whose `engine` is a real engine, but
	// not the one the TARGET session was created with.
	CodeEngineMismatch = "engine_mismatch"

	// CodeCapabilityNotSupported marks a pairing method the named engine does
	// not serve. No provider is called.
	CodeCapabilityNotSupported = "capability_not_supported"

	// CodeEngineUnavailable marks the inconsistency case: the capability
	// matrix says this engine serves the operation, and this process has no
	// provider wired for it.
	//
	// It is separate from capability_not_supported because the two need
	// opposite actions from whoever reads them. "Not supported" is permanent
	// and belongs to the caller — use another engine. "Unavailable" is a
	// deployment fact that belongs to whoever runs the process (headless
	// without Chrome configured is the shape this takes today, see
	// pkg/bootstrap/engine_headless.go), and it can be fixed without the
	// caller changing anything.
	CodeEngineUnavailable = "engine_unavailable"

	// CodeNoSession marks a target session id with no row behind it. Reused
	// verbatim from the code GetQRUseCase and CapabilityHandlers already
	// answer for the same condition, so a caller does not learn a second name
	// for one fact.
	CodeNoSession = "no_session"
)

// errInvalidEngine builds the 400 for a missing or unparseable engine.
func errInvalidEngine(raw string) error {
	return apperr.New(CodeInvalidEngine, apperr.CategoryValidation,
		fmt.Sprintf("engine must be %q or %q", domain.EngineWaNoise, domain.EngineWaHeadless),
		false, fmt.Errorf("%w (got %q)", domain.ErrInvalidEngine, raw))
}

// errEngineMismatch builds the 409 for a request that names the wrong engine
// for its target.
//
// The message names BOTH engines. Answering only "mismatch" would leave the
// caller guessing which of the two values to change, and the answer is never
// "change the session": engine is immutable after creation (F279).
func errEngineMismatch(requested, target domain.Engine) error {
	return apperr.New(CodeEngineMismatch, apperr.CategoryConflict,
		fmt.Sprintf("request names engine %q, but this session was created with engine %q; engine is immutable after creation",
			requested, target),
		false, nil)
}

// errCapabilityNotSupported builds the 422 for a pairing method the engine
// does not serve. reason carries the matrix's own note, which is where the
// evidence for the refusal is written down.
func errCapabilityNotSupported(capability domain.Capability, engine domain.Engine, status domain.CapabilityStatus, reason string) error {
	return apperr.New(CodeCapabilityNotSupported, apperr.CategoryCapabilityUnsupported,
		fmt.Sprintf("capability %q is %q on engine %q", capability, status, engine),
		false, fmt.Errorf("%s", reason))
}

// errEngineUnavailable builds the 409 for a provider the matrix promised and
// this process does not have.
func errEngineUnavailable(capability domain.Capability, engine domain.Engine) error {
	return apperr.New(CodeEngineUnavailable, apperr.CategoryConflict,
		fmt.Sprintf("engine %q is not available in this process for capability %q", engine, capability),
		false, nil)
}

// errNoSession builds the 400 for a target id with no row.
func errNoSession(txtID string) error {
	return apperr.New(CodeNoSession, apperr.CategoryValidation, "no session", false,
		fmt.Errorf("no user row for target session %q", txtID))
}
