// Package pairing adapts the wa_headless transport to the pairing surface
// (appport.SessionStarter, appport.PairingQRReader — pairphone is not here
// yet, see HOUSEKEEP F370 phase 2) — the wa_headless side of the same
// contract pkg/infra/wa-noise/adapters/pairing already serves for wa_noise.
package pairing

import (
	"context"

	"github.com/rs/zerolog/log"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/internal/wa-headless/capabilities/qr"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
)

const qrLabel = "adapter/qr"

// QRReader implements appport.PairingQRReader over a headless session.
type QRReader struct {
	sessions *adapter.Sessions
}

// NewQRReader builds the adapter.
func NewQRReader(sessions *adapter.Sessions) *QRReader {
	return &QRReader{sessions: sessions}
}

// EnsureSession reports whether this process can serve txtID, without
// booting.
func (r *QRReader) EnsureSession(ctx context.Context, txtID string) error {
	return r.sessions.EnsureSession(ctx, txtID)
}

// PairingQR returns the pairing code currently on offer for txtID.
//
// Not held (nobody called StartSession for this txtID, or it was released)
// answers ("", nil) without booting anything — the same "not connecting yet"
// shape GET /session/qr already tolerates for wa_noise. Held resolves the
// (already-booted, or reused) pairing session and reads the live QR string —
// see internal/wa-headless/capabilities/qr's own doc comment for where the
// construction comes from and what was measured; that package also nudges
// WAWebLaunchSocketUtils.refreshQR() on this call whenever Conn.ref is
// empty, so a caller polling this method already gets the auto-refresh
// behaviour for free.
//
// # Promotion out of the pairing quota
//
// An empty code is ALSO what a session that just finished pairing looks
// like (Conn.ref is cleared once authenticated), and registry.Promote has
// no caller in production without this: "the registry does not detect
// pairing itself... whoever observes it succeed calls this" (registry.go).
// So whenever the code comes back empty, PairingQR takes the one extra
// round trip to ask who the page is logged in as
// (waheadless.RefreshOwnIdentity, the same read GetStatus's headless
// SessionStatus already does) and promotes txtID out of the pairing quota
// the moment an identity is present — a poller that stops calling
// GET /session/qr right after seeing the account paired still leaves the
// session promoted, because this is the SAME call that noticed.
func (r *QRReader) PairingQR(ctx context.Context, txtID string) (string, error) {
	if !r.sessions.Holds(txtID) {
		return "", nil
	}
	eval, err := r.sessions.EvaluatorForPairing(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("wa_headless: pairing evaluator unavailable for QR read")
		return "", err
	}
	code, refreshed, err := qr.New(r.sessions.Runner(), eval).Read(ctx, qrLabel)
	if err != nil {
		return "", err
	}
	if code != "" {
		return code, nil
	}
	if refreshed {
		log.Info().Str("txt_id", txtID).Msg("wa_headless: nudged refreshQR, no code yet")
	}
	r.promoteIfPaired(ctx, txtID, eval)
	return "", nil
}

// promoteIfPaired checks whether txtID's page already has an owner
// identity and, if so, moves it from the pairing quota to the operational
// one. Best-effort: a failed identity read or a full operational pool
// (registry.Promote's own refusal) is logged and otherwise ignored — the
// NEXT poll tries again, and the session is never lost, only left in the
// pairing quota a little longer than ideal.
func (r *QRReader) promoteIfPaired(ctx context.Context, txtID string, eval waheadless.Evaluator) {
	identity, err := waheadless.RefreshOwnIdentity(ctx, r.sessions.Runner(), eval, qrLabel+"/owner-check")
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
