package bootstrap

import (
	"context"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	headlessadapter "wa-api/pkg/infra/headless"
	headlesspairing "wa-api/pkg/infra/headless/pairing"
	wapairing "wa-api/pkg/infra/noise/adapters/pairing"
	"wa-api/pkg/infra/noise/client"
	"wa-api/pkg/pairing"
)

// codeSessionOwnedByAnotherReplica is the apperr code a denied ownership claim
// answers with. Extracted from connectOwnershipCheck's inline literal when the
// check moved behind appport.SessionStarter: it is now written in one place and
// read in another, which is exactly the condition the project's
// zero-loose-literal rule (ADR-0004) exists for.
const codeSessionOwnedByAnotherReplica = "session_owned_by_another_replica"

// msgSessionOwnedByAnotherReplica is the historical message, byte for byte.
const msgSessionOwnedByAnotherReplica = "this session is owned by another replica; route the request to its owner"

// noiseSessionStarter implements appport.SessionStarter over the
// SessionOrchestrator that already drives the socket transport.
//
// It is a thin adapter and not new behaviour: both methods do exactly what the
// two closures injected into ConnectHandler did until 2026-08-27
// (WithStartSession / WithCheckOwnership). What changed is that they are now
// reached THROUGH an engine, so a session recorded as wa_headless cannot arrive
// here by default any more. See HOUSEKEEP F273/F281.
type noiseSessionStarter struct {
	server *server
}

// CheckOwnership claims the session lease for this process.
//
// F108: it runs SYNCHRONOUSLY, before the handler answers, because the claim
// used to happen inside the goroutine and a denial reached the client as
// 200 {"status":"connecting"}. The claim is idempotent for the same owner —
// StartSession's own Start claims it again and succeeds. In `single` mode
// s.Leases is nil, the claim always succeeds, and nothing is ever refused here.
func (s *noiseSessionStarter) CheckOwnership(_ context.Context, txtID string) error {
	if !claimSessionOwnership(s.server.Leases, txtID) {
		// claimSessionOwnership already logs WHY it refused (lease held
		// elsewhere, or the coordinator could not answer). This line says what
		// the refusal became: the request is about to be answered 409, which is
		// the fact the HTTP side is correlating against.
		log.Warn().Str("txt_id", txtID).Str("code", codeSessionOwnedByAnotherReplica).
			Msg("pairing: connect refused, session owned by another replica")
		return apperr.New(codeSessionOwnedByAnotherReplica, apperr.CategoryConflict,
			msgSessionOwnedByAnotherReplica, false, nil)
	}
	return nil
}

// StartSession launches the socket client in the background.
//
// The goroutine is HERE and not in the handler on purpose: fire-and-forget is a
// property of how this transport connects (the QR appears later, on
// GET /session/qr — see pkg/application/session/orchestrator.go), not a
// property of HTTP. An engine that connects synchronously would satisfy the
// same port without a goroutine, and the handler would not need to know.
//
// The request context is deliberately NOT propagated into the goroutine: the
// connection outlives the request that asked for it, and cancelling it when the
// client hangs up would tear down a session the client expects to find on its
// next poll.
func (s *noiseSessionStarter) StartSession(_ context.Context, txtID, token string) {
	go s.server.startSession(txtID, token)
}

var _ appport.SessionStarter = (*noiseSessionStarter)(nil)

// buildPairingRegistry wires the pairing surface: which provider serves which
// engine, for QR, phone code and connect.
//
// # wa_headless: all three pairing ports are wired (HOUSEKEEP F370/F380)
//
// Until 2026-08-29 (H145) wa_headless had an entry with three nil ports on
// purpose: no PairingQRReader, PhonePairer or SessionStarter implementation
// existed anywhere in the tree, and nothing under pkg/infra/wa-headless was
// even constructed in pkg/bootstrap. QR/connect were wired first —
// pkg/infra/wa-headless/pairing.QRReader/Starter, built over
// core.StartPairingSession (a new boot primitive; core.StartSession itself
// stays restoration-only) and the wwebjs-derived QR construction MEASURED
// against this build (internal/wa-headless/capabilities/qr, H145).
// PhonePairer followed the same day (F380): the same UNPAIRED-state gate
// H122 found for wa_noise's own phone pairing was measured against a
// genuinely unpaired session this time — H122 had only probed presence
// against an already-paired one, where the reference's own gate stops it
// before anything runs — and the call sequence
// (internal/wa-headless/capabilities/phonepair) reached WhatsApp's real
// server (a structured IQErrorBadRequest for a fake test number, not a
// crash).
//
// If a future engine still leaves this nil, that stays the same deliberate
// signal the original design chose over a missing entry
// (capability_not_supported vs engine_unavailable) — see
// pairing.Registry.ResolvePhonePairer.
func buildPairingRegistry(s *server, users appport.UserRepository, getClient client.Getter, caps *capabilityregistry.CapabilityRegistry, headlessSessions *headlessadapter.Sessions) *pairing.Registry {
	noise := &pairing.Provider{
		Engine:      domain.EngineNoise,
		QRReader:    wapairing.NewQRReaderAdapter(getClient, users),
		PhonePairer: wapairing.NewPhonePairerAdapter(getClient),
		Starter:     &noiseSessionStarter{server: s},
	}
	headless := &pairing.Provider{Engine: domain.EngineHeadless}
	headlessPorts := 0
	if headlessSessions != nil {
		headless.QRReader = headlesspairing.NewQRReader(headlessSessions)
		headless.Starter = headlesspairing.NewStarter(headlessSessions)
		headless.PhonePairer = headlesspairing.NewPhonePairer(headlessSessions)
		headlessPorts = 3
	}
	// Logged with the per-engine port counts rather than a bare "built": the
	// question an operator asks of this line is "does THIS process serve
	// headless pairing", and a count answers it where a provider list would
	// not.
	log.Info().
		Int("noise_ports", 3).
		Int("headless_ports", headlessPorts).
		Msg("pairing: provider registry wired")
	return pairing.NewRegistry(users, caps, noise, headless)
}
