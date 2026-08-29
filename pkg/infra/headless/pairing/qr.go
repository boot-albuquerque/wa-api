// Package pairing adapts the wa_headless transport to the pairing surface
// (appport.SessionStarter, appport.PairingQRReader — pairphone is not here
// yet, see HOUSEKEEP F370 phase 2) — the wa_headless side of the same
// contract pkg/infra/wa-noise/adapters/pairing already serves for wa_noise.
package pairing

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"wa-api/internal/headless"
	"wa-api/internal/headless/capabilities/qr"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/headless"
)

const qrLabel = "adapter/qr"

// QRReader implements appport.PairingQRReader over a headless session.
//
// codeSince tracks, per txtID, the assembled code currently on offer and
// when it FIRST became that code — the state qr.Reader itself cannot hold
// because a Reader is built fresh on every PairingQR call (see
// qr.StaleRefreshAfter's doc comment on why this layer, not that package,
// owns it). QRReader is a long-lived singleton (built once at wiring time,
// pkg/bootstrap/pairing_providers.go), so this map survives across the
// polls that come in as separate HTTP requests.
type QRReader struct {
	sessions *adapter.Sessions

	mu        sync.Mutex
	codeSince map[string]codeTrack
}

type codeTrack struct {
	code  string
	since time.Time
}

// NewQRReader builds the adapter.
func NewQRReader(sessions *adapter.Sessions) *QRReader {
	return &QRReader{sessions: sessions, codeSince: make(map[string]codeTrack)}
}

// staleHint reports whether the code currently on offer for txtID has been
// the SAME code for qr.StaleRefreshAfter or longer — the signal Read uses
// to force a nudge even though the page itself sees nothing wrong.
func (r *QRReader) staleHint(txtID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.codeSince[txtID]
	return ok && time.Since(t.since) >= qr.StaleRefreshAfter
}

// trackCode records the code just handed out for txtID, resetting the
// staleness clock only when the code actually CHANGED — re-stamping on
// every poll would mean "stale" never triggers, since the clock would
// never accumulate.
func (r *QRReader) trackCode(txtID, code string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.codeSince[txtID]; ok && prev.code == code {
		return
	}
	r.codeSince[txtID] = codeTrack{code: code, since: time.Now()}
}

// forgetCode drops txtID's tracked code — called whenever the session is no
// longer in a state where staleness means anything (not held, or pairing
// resolved to empty/paired), so a later pairing attempt for the same txtID
// starts its staleness clock fresh instead of inheriting a stale timestamp
// from a previous, unrelated pairing screen.
func (r *QRReader) forgetCode(txtID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.codeSince, txtID)
}

// EnsureSession reports whether this process can serve txtID, without
// booting.
func (r *QRReader) EnsureSession(ctx context.Context, txtID string) error {
	return r.sessions.EnsureSession(ctx, txtID)
}

// PairingQR returns the raw pairing code currently on offer for txtID.
//
// RAW, deliberately: the route this feeds — GET /session/pair/qr — is
// documented to answer a PNG data URI, but wa_noise's adapter returns one
// already (users.qrcode holds the rendered image) while this one has only
// the string the page produced. Normalising the two INSIDE each adapter is
// what the port looked like until F373, and it is how the two drifted apart
// unnoticed in the first place: both shapes are `string`, so nothing could
// tell them apart. The render now happens once, at the single point both
// engines pass through — GetQRUseCase, via qrimage.EnsureDataURI. See that
// use case's comment for the measurement.
//

// Not held (nobody called StartSession for this txtID, or it was released)
// answers ("", nil) without booting anything — the same "not connecting yet"
// shape GET /session/qr already tolerates for wa_noise. Held resolves the
// (already-booted, or reused) pairing session and reads the live QR string —
// see internal/wa-headless/capabilities/qr's own doc comment for where the
// construction comes from and what was measured; that package also nudges
// WAWebLaunchSocketUtils.refreshQR() on this call whenever Conn.ref is
// empty OR the page's own code has been on offer for qr.StaleRefreshAfter
// (this adapter's own codeSince tracks that — see its doc comment), so a
// caller polling this method already gets the auto-refresh behaviour for
// free either way.
//
// # Promotion out of the pairing quota
//
// An empty code is ALSO what a session that just finished pairing looks
// like (Conn.ref is cleared once authenticated), and registry.Promote has
// no caller in production without this: "the registry does not detect
// pairing itself... whoever observes it succeed calls this" (registry.go).
// So whenever the code comes back empty, PairingQR takes the one extra
// round trip to ask who the page is logged in as
// (headless.RefreshOwnIdentity, the same read GetStatus's headless
// SessionStatus already does) and promotes txtID out of the pairing quota
// the moment an identity is present — a poller that stops calling
// GET /session/qr right after seeing the account paired still leaves the
// session promoted, because this is the SAME call that noticed.
func (r *QRReader) PairingQR(ctx context.Context, txtID string) (string, error) {
	if !r.sessions.Holds(txtID) {
		r.forgetCode(txtID)
		return "", nil
	}
	eval, err := r.sessions.EvaluatorForPairing(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("wa_headless: pairing evaluator unavailable for QR read")
		return "", err
	}
	code, refreshed, err := qr.New(r.sessions.Runner(), eval).Read(ctx, qrLabel, r.staleHint(txtID))
	if err != nil {
		return "", err
	}
	if code != "" {
		r.trackCode(txtID, code)
		return code, nil
	}
	if refreshed {
		log.Info().Str("txt_id", txtID).Msg("wa_headless: nudged refreshQR, no code yet")
	}
	// Empty means either "not ready yet" or "just paired" — staleness is
	// meaningless in both, and a later pairing attempt for the same txtID
	// (a logout-then-reconnect) must not inherit a timestamp from before
	// this gap.
	r.forgetCode(txtID)
	r.promoteIfPaired(ctx, txtID, eval)
	return "", nil
}

// promoteIfPaired checks whether txtID's page already has an owner
// identity and, if so, moves it from the pairing quota to the operational
// one. Best-effort: a failed identity read or a full operational pool
// (registry.Promote's own refusal) is logged and otherwise ignored — the
// NEXT poll tries again, and the session is never lost, only left in the
// pairing quota a little longer than ideal.
func (r *QRReader) promoteIfPaired(ctx context.Context, txtID string, eval headless.Evaluator) {
	identity, err := headless.RefreshOwnIdentity(ctx, r.sessions.Runner(), eval, qrLabel+"/owner-check")
	if err != nil || !identity.Present() {
		return
	}
	if err := r.sessions.Promote(txtID); err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("wa_headless: pairing succeeded but promotion to operational failed")
		return
	}
	log.Info().Str("txt_id", txtID).Msg("wa_headless: pairing succeeded, promoted to operational")
}

var _ appport.PairingQRReader = (*QRReader)(nil)
