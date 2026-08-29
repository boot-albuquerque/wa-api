// Package pairing adapts the wa_headless transport to the pairing surface
// (appport.SessionStarter, appport.PairingQRReader — pairphone is not here
// yet, see HOUSEKEEP F370 phase 2) — the wa_headless side of the same
// contract pkg/infra/wa-noise/adapters/pairing already serves for wa_noise.
package pairing

import (
	"context"

	"github.com/rs/zerolog/log"

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
// construction comes from and what was measured.
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
	return qr.New(r.sessions.Runner(), eval).Read(ctx, qrLabel)
}

var _ appport.PairingQRReader = (*QRReader)(nil)
