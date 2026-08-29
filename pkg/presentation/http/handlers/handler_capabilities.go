package handlers

import (
	"context"
	"net/http"
	"sort"

	"github.com/rs/zerolog/hlog"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	customhttp "wa-api/pkg/presentation/http"
	dtocapability "wa-api/pkg/presentation/http/dto/capability"
)

// allCapabilities is every domain.Capability this build knows about — the
// VALUES of capabilityregistry.PortCoverage, deduplicated and sorted so the
// iteration order (and therefore any log line built from it) is stable
// across runs. PortCoverage, not domain.Capability's own constant block, is
// the source: it is what coverage_gate_test.go keeps in lockstep with the
// application ports, so a capability that exists here but has no port yet
// cannot silently appear in an admin/session response.
var allCapabilities = sortedCapabilities()

func sortedCapabilities() []domain.Capability {
	seen := make(map[domain.Capability]bool, len(capabilityregistry.PortCoverage))
	out := make([]domain.Capability, 0, len(capabilityregistry.PortCoverage))
	for _, c := range capabilityregistry.PortCoverage {
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// knownEngines is every engine the capability registry has a provider for.
// Sorted so /admin/capabilities has a stable row order.
var knownEngines = []domain.Engine{domain.EngineNoise, domain.EngineHeadless}

// knownAccountTypes is every account type the matrix is keyed on. Unlike
// knownEngines this deliberately INCLUDES AccountTypeUnknown: it is the only
// value account-type detection produces today (see CapabilityHandlers doc
// comment), so omitting it from /admin/capabilities would hide the one row
// that matters right now.
var knownAccountTypes = []domain.AccountType{
	domain.AccountTypePersonal, domain.AccountTypeBusiness, domain.AccountTypeUnknown,
}

// CapabilityHandlers groups GET /session/capabilities and GET
// /admin/capabilities.
//
// # account_type is always "unknown" today
//
// Real account-type detection is being built in a parallel worktree
// (account-type-detection) and is not available here. Every decision this
// handler asks the registry for uses domain.AccountTypeUnknown — which the
// registry treats as a legitimate, if less precise, answer (see
// domain.AccountType doc comment), not an error. Once detection lands
// upstream, SessionEngine below is the only place that needs to change: it
// resolves account_type the same way it resolves engine, from the session's
// persisted record.
type CapabilityHandlers struct {
	users    appport.UserRepository
	registry *capabilityregistry.CapabilityRegistry
}

// NewCapabilityHandlers builds the handler group.
func NewCapabilityHandlers(users appport.UserRepository, registry *capabilityregistry.CapabilityRegistry) *CapabilityHandlers {
	return &CapabilityHandlers{users: users, registry: registry}
}

// sessionEngine resolves the engine recorded for txtID via
// appport.UserRepository.ListUsers — the same read GetStatusUseCase already
// uses for the rest of the session record (pkg/application/usecase/session/
// get_status.go), so this does not open a second code path to the same
// column.
//
// A row whose engine still reads EngineLegacyUnknown (column added but the
// startup backfill has not run against it) falls back to EngineNoise, the
// documented default the backfill itself would assign
// (pkg/infra/db/user_engine.go) — a handler must not surface an internal
// migration state as a client-facing error.
func (h *CapabilityHandlers) sessionEngine(ctx context.Context, txtID string) (domain.Engine, error) {
	entries, err := h.users.ListUsers(ctx, txtID)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", apperr.New("no_session", apperr.CategoryValidation, "no session", false, nil)
	}
	engine := entries[0].Engine
	if !engine.IsValidForCreate() {
		return domain.EngineNoise, nil
	}
	return engine, nil
}

// Session handles GET /session/capabilities: what the AUTHENTICATED
// session's engine can do, one status per capability — no evidence/reason,
// per item 92 of the architectural prompt ("business knows capability,
// admin knows engine").
func (h *CapabilityHandlers) Session(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}

	engine, err := h.sessionEngine(r.Context(), id)
	if err != nil {
		if isClientCausedSessionError(err) {
			hlog.FromRequest(r).Warn().Err(err).Str("handler", "SessionCapabilities").Str("user_id", id).
				Msg("session capabilities read rejected")
			customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		hlog.FromRequest(r).Error().Err(err).Str("handler", "SessionCapabilities").Str("user_id", id).
			Msg("session capabilities read failed")
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}

	// See the CapabilityHandlers doc comment: unknown until account-type
	// detection lands.
	accountType := domain.AccountTypeUnknown

	caps := make(map[string]string, len(allCapabilities))
	for _, c := range allCapabilities {
		decision, err := h.registry.Decide(c, engine, accountType)
		if err != nil {
			// Only reachable if engine somehow stopped being one of the two
			// the registry knows — sessionEngine above already guards
			// against that. Fail the single capability as unknown instead
			// of the whole response, and log loud enough to notice.
			hlog.FromRequest(r).Error().Err(err).Str("handler", "SessionCapabilities").
				Str("capability", c.String()).Str("engine", engine.String()).
				Msg("capability decision failed for a known engine")
			caps[c.String()] = domain.StatusUnknown.String()
			continue
		}
		caps[c.String()] = decision.Status.String()
	}

	customhttp.RespondJSON(w, http.StatusOK, dtocapability.PresentSessionCapabilities(accountType.String(), caps), nil)
}

// Admin handles GET /admin/capabilities: the full capability × engine ×
// account_type matrix, unfiltered by session. Protected by authAdmin at the
// router level (same subrouter as every other /admin/* route) — this
// handler does not re-check the admin token.
func (h *CapabilityHandlers) Admin(w http.ResponseWriter, r *http.Request) {
	rows := make([]dtocapability.AdminCapabilityRow, 0, len(allCapabilities)*len(knownEngines)*len(knownAccountTypes))
	for _, c := range allCapabilities {
		for _, engine := range knownEngines {
			for _, accountType := range knownAccountTypes {
				decision, err := h.registry.Decide(c, engine, accountType)
				if err != nil {
					// Same defensive stance as Session: never let one bad
					// combination take the whole matrix down.
					hlog.FromRequest(r).Error().Err(err).Str("handler", "AdminCapabilities").
						Str("capability", c.String()).Str("engine", engine.String()).
						Str("account_type", accountType.String()).
						Msg("capability decision failed while building admin matrix")
					rows = append(rows, dtocapability.AdminCapabilityRow{
						Capability:  c.String(),
						Engine:      engine.String(),
						AccountType: accountType.String(),
						Status:      domain.StatusUnknown.String(),
						Evidence:    domain.EvidenceUnknown.String(),
						Reason:      err.Error(),
					})
					continue
				}
				rows = append(rows, dtocapability.AdminCapabilityRow{
					Capability:            decision.Capability.String(),
					Engine:                decision.Engine.String(),
					AccountType:           decision.AccountType.String(),
					Supported:             decision.Supported,
					Status:                decision.Status.String(),
					Reason:                decision.Reason,
					Evidence:              decision.Evidence.String(),
					RequiredPreconditions: decision.RequiredPreconditions,
				})
			}
		}
	}

	customhttp.RespondJSON(w, http.StatusOK, dtocapability.PresentAdminCapabilities(rows), nil)
}
