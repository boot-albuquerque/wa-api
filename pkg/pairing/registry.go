package pairing

import (
	"context"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
)

// Provider is everything ONE engine offers on the pairing surface.
//
// The three ports are nullable, and a nil one is a statement: this engine has
// no adapter for that operation in this build. It is never a reason to reach
// for another engine's — see the package doc on why there is no fallback.
//
// A nil port and an unsupported capability are answered differently
// (engine_unavailable vs capability_not_supported) because they are different
// facts, and the capability matrix is the one that decides which question gets
// asked first.
type Provider struct {
	// Engine identifies the transport this provider serves.
	Engine domain.Engine

	// QRReader serves GET /session/qr. Nil when this engine offers no QR.
	QRReader appport.PairingQRReader

	// PhonePairer serves POST /session/pairphone. Nil when this engine has no
	// phone-code pairing.
	PhonePairer appport.PhonePairer

	// Starter serves GET /session/connect. Nil when this engine cannot start
	// a session from an HTTP request in this build.
	Starter appport.SessionStarter
}

// SessionEngineReader reads the engine RECORDED for a session.
//
// It is a one-method view over appport.UserRepository rather than the whole
// interface, because this package must not be able to write: resolving which
// engine serves a request has no business updating the row that answers it.
type SessionEngineReader interface {
	// ListUsers returns the row for id, or an empty slice when there is none.
	ListUsers(ctx context.Context, id string) ([]domain.UserListEntry, error)
}

// Registry resolves a pairing request to the provider that must serve it.
type Registry struct {
	providers map[domain.Engine]*Provider
	sessions  SessionEngineReader
	caps      *capabilityregistry.CapabilityRegistry
}

// NewRegistry builds the router over the providers this process has.
//
// caps is the SHARED capabilityregistry.CapabilityRegistry — the same instance
// GET /session/capabilities answers from. That is deliberate and is the whole
// reason this package does not carry a capability table of its own: two tables
// would let /session/capabilities promise something the pairing path refuses,
// which is the exact class of divergence the registry was built to end.
func NewRegistry(sessions SessionEngineReader, caps *capabilityregistry.CapabilityRegistry, providers ...*Provider) *Registry {
	byEngine := make(map[domain.Engine]*Provider, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		byEngine[p.Engine] = p
	}
	log.Info().Int("providers", len(byEngine)).Msg("pairing: provider registry built")
	return &Registry{providers: byEngine, sessions: sessions, caps: caps}
}

// TargetEngine reports the engine RECORDED for the target session.
//
// Exported because it is the one fact a caller may legitimately want without
// resolving a provider (a log line, a diagnostic), and because naming it makes
// the actor/target distinction visible at every call site: there is no
// ActorEngine, and there is not going to be one.
//
// A row whose engine reads legacy_unknown is reported as wa_noise, matching
// what the startup backfill would assign (pkg/infra/db/user_engine.go) and what
// CapabilityHandlers.sessionEngine already does for the same state. Surfacing an
// internal migration state to a client would be a worse answer than the default
// the migration itself picks.
func (r *Registry) TargetEngine(ctx context.Context, targetSessionID string) (domain.Engine, error) {
	entries, err := r.sessions.ListUsers(ctx, targetSessionID)
	if err != nil {
		log.Error().Err(err).Str("target_session_id", targetSessionID).
			Msg("pairing: could not read target session engine")
		return "", err
	}
	if len(entries) == 0 {
		log.Warn().Str("target_session_id", targetSessionID).
			Msg("pairing: no row for target session")
		return "", errNoSession(targetSessionID)
	}
	engine := entries[0].Engine
	if !engine.IsValidForCreate() {
		return domain.EngineWaNoise, nil
	}
	return engine, nil
}

// Resolve answers the four questions of the pairing surface, in the order the
// package doc fixes, and returns the provider that must serve the request.
//
// requestedEngine is the RAW text from the request — query parameter or JSON
// field — and is parsed here rather than by the caller, so that "missing",
// "empty" and "not an engine" cannot end up with three different answers in
// three handlers.
//
// No provider is touched on any refusal path. That is the property the spies in
// registry_test.go assert, and it is the one that matters: the alternative is
// discovering a request should have been rejected after having already driven
// somebody's WhatsApp session over the wrong transport.
func (r *Registry) Resolve(ctx context.Context, targetSessionID, requestedEngine string, capability domain.Capability) (*Provider, error) {
	engine, err := domain.ParseEngine(requestedEngine)
	if err != nil {
		log.Warn().Str("target_session_id", targetSessionID).
			Str("capability", capability.String()).
			Msg("pairing: request rejected — engine missing or not an engine")
		return nil, errInvalidEngine(requestedEngine)
	}

	targetEngine, err := r.TargetEngine(ctx, targetSessionID)
	if err != nil {
		return nil, err
	}
	if engine != targetEngine {
		log.Warn().Str("target_session_id", targetSessionID).
			Str("requested_engine", engine.String()).
			Str("target_engine", targetEngine.String()).
			Str("capability", capability.String()).
			Msg("pairing: request rejected — requested engine is not the target session's engine")
		return nil, errEngineMismatch(engine, targetEngine)
	}

	// account_type is unknown for the same reason it is unknown in
	// CapabilityHandlers: detection is not wired yet. The registry treats it
	// as a legitimate value, not an error.
	decision, err := r.caps.Decide(capability, engine, domain.AccountTypeUnknown)
	if err != nil {
		log.Error().Err(err).Str("engine", engine.String()).Str("capability", capability.String()).
			Msg("pairing: capability decision failed for an engine the parser accepted")
		return nil, errEngineUnavailable(capability, engine)
	}
	if !decision.Supported {
		log.Warn().Str("target_session_id", targetSessionID).
			Str("engine", engine.String()).Str("capability", capability.String()).
			Str("status", decision.Status.String()).
			Msg("pairing: request rejected — engine does not serve this capability")
		return nil, errCapabilityNotSupported(capability, engine, decision.Status, decision.Reason)
	}

	provider, ok := r.providers[engine]
	if !ok || provider == nil {
		log.Error().Str("engine", engine.String()).Str("capability", capability.String()).
			Msg("pairing: matrix promises this capability and no provider is wired for the engine")
		return nil, errEngineUnavailable(capability, engine)
	}
	return provider, nil
}

// ResolveQRReader resolves GET /session/qr.
func (r *Registry) ResolveQRReader(ctx context.Context, targetSessionID, requestedEngine string) (appport.PairingQRReader, error) {
	provider, err := r.Resolve(ctx, targetSessionID, requestedEngine, domain.CapGetPairingQR)
	if err != nil {
		return nil, err
	}
	if provider.QRReader == nil {
		log.Error().Str("engine", provider.Engine.String()).
			Msg("pairing: provider has no QR reader although the matrix promises one")
		return nil, errEngineUnavailable(domain.CapGetPairingQR, provider.Engine)
	}
	return provider.QRReader, nil
}

// ResolvePhonePairer resolves POST /session/pairphone.
func (r *Registry) ResolvePhonePairer(ctx context.Context, targetSessionID, requestedEngine string) (appport.PhonePairer, error) {
	provider, err := r.Resolve(ctx, targetSessionID, requestedEngine, domain.CapRequestPairingCode)
	if err != nil {
		return nil, err
	}
	if provider.PhonePairer == nil {
		log.Error().Str("engine", provider.Engine.String()).
			Msg("pairing: provider has no phone pairer although the matrix promises one")
		return nil, errEngineUnavailable(domain.CapRequestPairingCode, provider.Engine)
	}
	return provider.PhonePairer, nil
}

// ResolveStarter resolves GET /session/connect.
func (r *Registry) ResolveStarter(ctx context.Context, targetSessionID, requestedEngine string) (appport.SessionStarter, error) {
	provider, err := r.Resolve(ctx, targetSessionID, requestedEngine, domain.CapConnectSession)
	if err != nil {
		return nil, err
	}
	if provider.Starter == nil {
		log.Error().Str("engine", provider.Engine.String()).
			Msg("pairing: provider has no session starter although the matrix promises one")
		return nil, errEngineUnavailable(domain.CapConnectSession, provider.Engine)
	}
	return provider.Starter, nil
}
