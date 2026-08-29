package bootstrap

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/pairing"
)

// codeSessionEngineUnavailable marks the case where a session is recorded
// under an engine this process has no SessionController wired for. Named
// distinctly from pairing's own CodeEngineUnavailable (pkg/pairing/errors.go)
// because the two packages must stay decodable independently — a caller
// reading one should not have to know the other exists.
const codeSessionEngineUnavailable = "session_engine_unavailable"

// codeSessionCapabilityNotSupported marks a Disconnect/Logout call the
// capability matrix does not mark as supported for the target's recorded
// engine. Named distinctly from pairing's own CodeCapabilityNotSupported
// (pkg/pairing/errors.go) for the same reason as
// codeSessionEngineUnavailable above.
const codeSessionCapabilityNotSupported = "session_capability_not_supported"

// sessionEngineGuard dispatches the four SessionController operations
// (EnsureSession, Disconnect, Logout, SessionStatus) to the adapter of the
// engine RECORDED for the target session, instead of always driving wa_noise.
//
// # Why this exists
//
// Until this type, every consumer of the package-level `sessionGuard` value
// in wiring_handlers.go — DisconnectUseCase, LogoutUseCase, GetStatusUseCase,
// and eight other use cases that only call EnsureSession (S3/HMAC/proxy/
// history config, ListUsers, DeleteUserComplete) — was wired directly to
// wasession.NewSessionGuardAdapter(waClientLookup), which only knows
// wa_noise. A session recorded as wa_headless failed EnsureSession in every
// one of those, not only in /session/disconnect|logout|status. This type is
// a drop-in replacement for that single value: it implements
// appport.SessionController, so every existing call site keeps compiling
// unchanged.
//
// # Why it mirrors pairing.Registry.TargetEngine instead of importing it
//
// pkg/pairing.Registry resolves a REQUESTED engine against a target's
// RECORDED engine (the request must name it explicitly — see
// pkg/pairing/registry.go's own doc). None of EnsureSession/Disconnect/
// Logout/SessionStatus carry a requested-engine parameter in their existing
// signatures (appport.SessionController), and adding one would ripple into
// eleven use cases that have no reason to know engines exist. So this type
// only answers "which engine is THIS session recorded under", the same
// lookup pairing.Registry.TargetEngine does over the same repository method,
// duplicated rather than shared because the two packages must not need to
// import each other for a three-line lookup.
type sessionEngineGuard struct {
	// users is deliberately pairing.SessionEngineReader — the same
	// one-method view pkg/pairing.Registry already resolves its own
	// TargetEngine through — rather than the full appport.UserRepository:
	// this type has no business writing the row that answers it either.
	users pairing.SessionEngineReader
	caps  *capabilityregistry.CapabilityRegistry

	waNoise appport.SessionController

	// waHeadlessDisconnector is nil when this process has no Chrome
	// configured for wa_headless. A nil value is a valid, meaningful state:
	// it means this process has no headless session controller right now,
	// not that headless sessions cannot exist.
	waHeadlessDisconnector appport.SessionDisconnector

	// waHeadlessLogouter stays nil until Socket.logout's call shape is
	// measured against a real page (see
	// pkg/infra/wa-headless/session/disconnector.go's package doc) — a
	// separate field, not the same object typed down, because
	// SessionDisconnector existing does not imply SessionLogouter does: the
	// two are deliberately decoupled capabilities (session_guard.go).
	waHeadlessLogouter appport.SessionLogouter
}

// newSessionEngineGuard builds the dispatcher. Both waHeadless* may be nil.
func newSessionEngineGuard(users pairing.SessionEngineReader, caps *capabilityregistry.CapabilityRegistry, waNoise appport.SessionController, waHeadlessDisconnector appport.SessionDisconnector, waHeadlessLogouter appport.SessionLogouter) *sessionEngineGuard {
	return &sessionEngineGuard{
		users:                  users,
		caps:                   caps,
		waNoise:                waNoise,
		waHeadlessDisconnector: waHeadlessDisconnector,
		waHeadlessLogouter:     waHeadlessLogouter,
	}
}

// targetEngine reports the engine recorded for txtID.
//
// A row whose engine reads legacy_unknown resolves to wa_noise, matching the
// startup backfill (pkg/infra/db/user_engine.go) and pairing.Registry's own
// fallback (pkg/pairing/registry.go:100-102) — surfacing the migration state
// itself would be a worse answer than the default the migration picks.
func (g *sessionEngineGuard) targetEngine(ctx context.Context, txtID string) (domain.Engine, error) {
	entries, err := g.users.ListUsers(ctx, txtID)
	if err != nil {
		log.Error().Err(err).Str("txt_id", txtID).
			Msg("session_engine_guard: could not read target session engine")
		return "", err
	}
	if len(entries) == 0 {
		// No row: callers of EnsureSession/Disconnect/Logout already treat
		// "no session" as their own condition (e.g. GetStatusUseCase reads
		// the row separately). Defaulting to wa_noise here matches the
		// backfill default and lets the underlying wa_noise adapter produce
		// its own, already-tested "no session" error rather than inventing a
		// second one.
		return domain.EngineWaNoise, nil
	}
	engine := entries[0].Engine
	if !engine.IsValidForCreate() {
		return domain.EngineWaNoise, nil
	}
	return engine, nil
}

// errEngineUnavailable builds the refusal for a resolved engine this process
// has no adapter wired for. Logging the refusal is the CALLER's job
// (disconnectorFor/logouterFor), matching pkg/pairing/errors.go's own split
// between a Debug taxonomy line here and a Warn operational line at the
// resolve call site — logging here too would double the line for one event.
func errEngineUnavailable(engine domain.Engine) error {
	log.Debug().Str("code", codeSessionEngineUnavailable).Str("engine", engine.String()).
		Msg("session_engine_guard: building refusal")
	return apperr.New(codeSessionEngineUnavailable, apperr.CategoryConflict,
		fmt.Sprintf("engine %q is not available in this process", engine), false, nil)
}

// disconnectorFor resolves the SessionDisconnector for engine (EnsureSession,
// Disconnect, SessionStatus all live on this narrower interface — see
// session_guard.go).
func (g *sessionEngineGuard) disconnectorFor(engine domain.Engine) (appport.SessionDisconnector, error) {
	var d appport.SessionDisconnector
	switch engine {
	case domain.EngineWaNoise:
		d = g.waNoise
	case domain.EngineWaHeadless:
		d = g.waHeadlessDisconnector
	}
	if d == nil {
		log.Warn().Str("engine", engine.String()).
			Msg("session_engine_guard: no SessionDisconnector wired for this engine")
		return nil, errEngineUnavailable(engine)
	}
	return d, nil
}

// logouterFor resolves the SessionLogouter for engine.
func (g *sessionEngineGuard) logouterFor(engine domain.Engine) (appport.SessionLogouter, error) {
	var l appport.SessionLogouter
	switch engine {
	case domain.EngineWaNoise:
		l = g.waNoise
	case domain.EngineWaHeadless:
		l = g.waHeadlessLogouter
	}
	if l == nil {
		log.Warn().Str("engine", engine.String()).
			Msg("session_engine_guard: no SessionLogouter wired for this engine")
		return nil, errEngineUnavailable(engine)
	}
	return l, nil
}

// checkCapability refuses when the capability matrix does not mark
// capability as supported for engine. Only Disconnect and Logout call this —
// EnsureSession and SessionStatus are diagnostic/guard operations, not
// capabilities a caller requests, so they dispatch on engine availability
// alone.
func (g *sessionEngineGuard) checkCapability(capability domain.Capability, engine domain.Engine) error {
	decision, err := g.caps.Decide(capability, engine, domain.AccountTypeUnknown)
	if err != nil {
		log.Error().Err(err).Str("capability", capability.String()).Str("engine", engine.String()).
			Msg("session_engine_guard: capability decision failed for a known engine")
		return apperr.New(codeSessionEngineUnavailable, apperr.CategoryConflict,
			fmt.Sprintf("capability %q could not be resolved for engine %q", capability, engine), false, err)
	}
	if !decision.Supported {
		return apperr.New(codeSessionCapabilityNotSupported, apperr.CategoryCapabilityUnsupported,
			fmt.Sprintf("capability %q is %q on engine %q", capability, decision.Status, engine),
			false, fmt.Errorf("%s", decision.Reason))
	}
	return nil
}

// EnsureSession devolve nil se há sessão utilizável para txtID no engine
// gravado, e erro caso contrário.
func (g *sessionEngineGuard) EnsureSession(ctx context.Context, txtID string) error {
	engine, err := g.targetEngine(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("session_engine_guard: EnsureSession could not resolve target engine")
		return err
	}
	disconnector, err := g.disconnectorFor(engine)
	if err != nil {
		return err
	}
	err = disconnector.EnsureSession(ctx, txtID)
	return err
}

// Disconnect derruba o transporte da sessão, no engine gravado.
func (g *sessionEngineGuard) Disconnect(ctx context.Context, txtID string) error {
	engine, err := g.targetEngine(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("session_engine_guard: Disconnect could not resolve target engine")
		return err
	}
	if err := g.checkCapability(domain.CapDisconnectSession, engine); err != nil {
		return err
	}
	disconnector, err := g.disconnectorFor(engine)
	if err != nil {
		return err
	}
	err = disconnector.Disconnect(ctx, txtID)
	return err
}

// Logout desautentica a sessão no WhatsApp, no engine gravado.
func (g *sessionEngineGuard) Logout(ctx context.Context, txtID string) error {
	engine, err := g.targetEngine(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("session_engine_guard: Logout could not resolve target engine")
		return err
	}
	if err := g.checkCapability(domain.CapLogoutSession, engine); err != nil {
		return err
	}
	logouter, err := g.logouterFor(engine)
	if err != nil {
		return err
	}
	err = logouter.Logout(ctx, txtID)
	return err
}

// SessionStatus informa o estado ao vivo da sessão, no engine gravado.
//
// Sem erro na assinatura (appport.SessionStatusReader), então qualquer
// falha de resolução — sem linha, sem adapter para o engine — devolve
// (false, false) em vez de propagar: é a mesma resposta honesta que
// GetStatusUseCase já dá para "não há sessão viva agora".
func (g *sessionEngineGuard) SessionStatus(ctx context.Context, txtID string) (connected, loggedIn bool) {
	engine, err := g.targetEngine(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("session_engine_guard: SessionStatus could not resolve target engine, reporting disconnected")
		return false, false
	}
	disconnector, err := g.disconnectorFor(engine)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).Str("engine", engine.String()).
			Msg("session_engine_guard: SessionStatus has no adapter for this engine, reporting disconnected")
		return false, false
	}
	return disconnector.SessionStatus(ctx, txtID)
}

var _ appport.SessionController = (*sessionEngineGuard)(nil)
