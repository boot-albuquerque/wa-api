package capabilityregistry

import (
	"fmt"

	"wa-api/pkg/domain"

	"github.com/rs/zerolog/log"
)

// CapabilityRegistry resolves which CapabilityProvider serves an engine, and
// is the entry point that also folds in RUNTIME preconditions a caller has
// already checked (permission, account state) — the static matrix alone only
// knows engine × account_type.
//
// It does NOT decide business rules and does NOT grow into a God Object: the
// only things it does are (1) look up a provider by engine and (2) merge a
// static decision with caller-supplied runtime facts. Anything more belongs
// in the use case that calls it.
type CapabilityRegistry struct {
	providers map[string]CapabilityProvider
}

// NewCapabilityRegistry builds a registry with one provider per known
// engine, backed by the shared default matrix.
func NewCapabilityRegistry() *CapabilityRegistry {
	m := NewDefaultMatrix()
	return &CapabilityRegistry{
		providers: map[string]CapabilityProvider{
			domain.EngineNoise:      &matrixProvider{engine: domain.EngineNoise, m: m},
			domain.EngineWaHeadless: &matrixProvider{engine: domain.EngineWaHeadless, m: m},
		},
	}
}

// ErrUnknownEngine is returned by Provider/Decide when asked about an engine
// this registry has no provider for (i.e. not domain.EngineNoise or
// domain.EngineWaHeadless).
var ErrUnknownEngine = fmt.Errorf("capabilityregistry: unknown engine")

// Provider resolves the CapabilityProvider for one engine.
func (r *CapabilityRegistry) Provider(engine string) (CapabilityProvider, error) {
	p, ok := r.providers[engine]
	if !ok {
		log.Warn().Str("engine", engine).
			Msg("capabilityregistry: no provider for engine")
		return nil, fmt.Errorf("%w: %q", ErrUnknownEngine, engine)
	}
	return p, nil
}

// Decide resolves one (capability, engine, account_type) against the static
// matrix, then folds in failedPreconditions the CALLER has already
// evaluated against live session state (e.g. "not group admin"). The
// registry itself never inspects session state — it has none — so a nil or
// empty failedPreconditions means "the caller did not check any," not "all
// preconditions hold."
//
// When the static decision is already unsupported, failedPreconditions is
// still attached (a caller may want to log both reasons), but Supported
// stays false regardless — a satisfied precondition can never override an
// engine/account_type gap, only add detail to it.
func (r *CapabilityRegistry) Decide(capability domain.Capability, engine string, accountType domain.AccountType, failedPreconditions ...string) (CapabilityDecision, error) {
	p, err := r.Provider(engine)
	if err != nil {
		return CapabilityDecision{}, err
	}
	d := p.Supports(capability, accountType)
	if len(failedPreconditions) > 0 {
		d.FailedPreconditions = failedPreconditions
		if d.Supported {
			d.Supported = false
			d.Status = domain.StatusPermissionRequired
			d.Reason = fmt.Sprintf("capability %q on engine %q is otherwise supported, but these preconditions failed: %v", capability, engine, failedPreconditions)
			log.Warn().Str("capability", capability.String()).Str("engine", engine).
				Strs("failed_preconditions", failedPreconditions).
				Msg("capabilityregistry: downgraded supported decision to permission_required")
		}
	}
	return d, nil
}
