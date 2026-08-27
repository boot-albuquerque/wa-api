package bootstrap

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/capabilityregistry"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	wapairing "wa-api/pkg/infra/wa-noise/adapters/pairing"
	waclient "wa-api/pkg/infra/wa-noise/client"
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

// waNoiseSessionStarter implements appport.SessionStarter over the
// SessionOrchestrator that already drives the socket transport.
//
// It is a thin adapter and not new behaviour: both methods do exactly what the
// two closures injected into ConnectHandler did until 2026-08-27
// (WithStartSession / WithCheckOwnership). What changed is that they are now
// reached THROUGH an engine, so a session recorded as wa_headless cannot arrive
// here by default any more. See HOUSEKEEP F273/F281.
type waNoiseSessionStarter struct {
	server *server
}

// CheckOwnership claims the session lease for this process.
//
// F108: it runs SYNCHRONOUSLY, before the handler answers, because the claim
// used to happen inside the goroutine and a denial reached the client as
// 200 {"status":"connecting"}. The claim is idempotent for the same owner —
// StartSession's own Start claims it again and succeeds. In `single` mode
// s.Leases is nil, the claim always succeeds, and nothing is ever refused here.
func (s *waNoiseSessionStarter) CheckOwnership(_ context.Context, txtID string) error {
	if !claimSessionOwnership(s.server.Leases, txtID) {
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
func (s *waNoiseSessionStarter) StartSession(_ context.Context, txtID, token string) {
	go s.server.startSession(txtID, token)
}

var _ appport.SessionStarter = (*waNoiseSessionStarter)(nil)

// buildPairingRegistry wires the pairing surface: which provider serves which
// engine, for QR, phone code and connect.
//
// # Why wa_headless has an entry with three nil ports, instead of no entry
//
// A missing entry and an entry with nothing in it produce different errors, and
// the difference is the one an operator needs. No entry at all would mean this
// process does not know the engine exists; an empty provider says it knows and
// has nothing wired for it. Today neither is reachable from a request, because
// the capability matrix refuses wa_headless for all three operations first
// (pkg/capabilityregistry/matrix.go: get_pairing_qr and connect_session are
// not_implemented there, request_pairing_code is unknown) — the entry exists so
// that the day a headless adapter lands, wiring it is filling in a field rather
// than discovering that the registration site was never written.
//
// This is measured, not assumed: as of 2026-08-27 nothing under
// pkg/infra/wa-headless is constructed anywhere in pkg/bootstrap, and no
// PairingQRReader, PhonePairer or SessionStarter implementation exists for that
// engine anywhere in the tree.
func buildPairingRegistry(s *server, users appport.UserRepository, getClient waclient.Getter, caps *capabilityregistry.CapabilityRegistry) *pairing.Registry {
	waNoise := &pairing.Provider{
		Engine:      domain.EngineWaNoise,
		QRReader:    wapairing.NewQRReaderAdapter(getClient, users),
		PhonePairer: wapairing.NewPhonePairerAdapter(getClient),
		Starter:     &waNoiseSessionStarter{server: s},
	}
	waHeadless := &pairing.Provider{Engine: domain.EngineWaHeadless}
	return pairing.NewRegistry(users, caps, waNoise, waHeadless)
}
