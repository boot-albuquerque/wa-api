package pairing

import (
	"context"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/headless"
)

// Starter implements appport.SessionStarter over a headless session: it
// begins booting a pairing session, fire-and-forget — the QR appears later,
// read by QRReader, matching the historical noise contract
// (pkg/bootstrap/pairing_providers.go's own doc on why StartSession returns
// nothing).
type Starter struct {
	sessions *adapter.Sessions
}

// NewStarter builds the adapter.
func NewStarter(sessions *adapter.Sessions) *Starter {
	return &Starter{sessions: sessions}
}

// CheckOwnership reports whether this process may drive txtID.
//
// noise's equivalent (noiseSessionStarter.CheckOwnership,
// pairing_providers.go) claims a cross-replica LEASE, because the socket
// transport can be driven from any replica that holds the lease. headless
// has no such thing to check yet: a Chrome profile is local disk, not a
// resource a lease coordinates across replicas, and registry.Acquire already
// refuses a SECOND concurrent boot of the same profile within this one
// process (registry.go). Multi-replica headless deployment — the day a
// profile could plausibly be reachable from more than one process — is out
// of scope until that day defines what "ownership" even means for it.
func (s *Starter) CheckOwnership(_ context.Context, _ string) error {
	return nil
}

// StartSession begins the pairing boot for txtID.
//
// Fire-and-forget by contract, same as noise's Starter
// (appport.SessionStarter's own doc: the QR appears later, on
// GET /session/qr). The request context is deliberately NOT propagated into
// the goroutine — the session outlives the request that asked for it.
func (s *Starter) StartSession(_ context.Context, txtID, _ string) {
	go func() {
		if _, err := s.sessions.EvaluatorForPairing(context.Background(), txtID); err != nil {
			log.Warn().Err(err).Str("txt_id", txtID).
				Msg("headless: pairing session failed to start")
		}
	}()
}

var _ appport.SessionStarter = (*Starter)(nil)
