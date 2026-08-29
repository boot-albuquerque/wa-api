package pairing

import (
	"context"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/internal/wa-headless/capabilities/phonepair"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
	adapter "wa-api/pkg/infra/wa-headless"
)

const phonePairLabel = "adapter/phonepair"

// codePairingFailedCode is the SAME apperr code
// pkg/infra/wa-noise/adapters/pairing/adapter.go uses for the identical
// condition — a pairing-code request WhatsApp itself refused. wa_noise's
// own comment there documents this as deliberate contract fidelity (F152):
// GET /session/pairphone answers 400 for every pairing failure, whether the
// cause is a malformed phone number or the server's own refusal. Verified
// live (HOUSEKEEP F380): a fake test number produced WhatsApp's
// IQErrorBadRequest, and this wrapping is what turns that into the 400 a
// caller of either engine already expects — the same shape by construction
// (const, not a repeated literal — ADR-0004), not by coincidence.
const codePairingFailedCode = "pair_phone_failed"

// PhonePairer implements appport.PhonePairer over a headless session — the
// wa_headless side of the same contract
// pkg/infra/wa-noise/adapters/pairing.PhonePairerAdapter already serves for
// wa_noise. See internal/wa-headless/capabilities/phonepair's own doc
// comment for where the call sequence comes from and what was measured
// (HOUSEKEEP F380).
type PhonePairer struct {
	sessions *adapter.Sessions
}

// NewPhonePairer builds the adapter.
func NewPhonePairer(sessions *adapter.Sessions) *PhonePairer {
	return &PhonePairer{sessions: sessions}
}

// EnsureSession reports whether this process can serve txtID, without
// booting — same contract as QRReader.EnsureSession.
func (p *PhonePairer) EnsureSession(ctx context.Context, txtID string) error {
	return p.sessions.EnsureSession(ctx, txtID)
}

// IsPaired reports whether txtID's page already has an owner identity —
// same read PairingQR's promoteIfPaired and Disconnector.SessionStatus
// already do (waheadless.RefreshOwnIdentity). PairPhoneUseCase calls this
// BEFORE RequestPairingCode (appport.PhonePairer's own doc comment): asking
// for a code on an already-paired session is meaningless, and WhatsApp's
// own state gate would refuse it anyway (HOUSEKEEP H122/F380 — the
// reference's requestPairingCode only proceeds while
// WAWebSocketModel.Socket.state is UNPAIRED/UNPAIRED_IDLE).
//
// Not held answers (false, nil): a session nobody started pairing for
// cannot be paired either, and this must not boot a browser just to answer
// that.
func (p *PhonePairer) IsPaired(ctx context.Context, txtID string) (bool, error) {
	if !p.sessions.Holds(txtID) {
		return false, nil
	}
	eval, err := p.sessions.EvaluatorForPairing(ctx, txtID)
	if err != nil {
		return false, err
	}
	identity, err := waheadless.RefreshOwnIdentity(ctx, p.sessions.Runner(), eval, phonePairLabel)
	if err != nil {
		// owner.ErrNoOwner (showing QR/pairing screen, no owner yet) and any
		// read failure alike mean "not confirmed paired" — the same
		// tolerant shape PairingQR's own promotion check uses.
		return false, nil
	}
	return identity.Present(), nil
}

// RequestPairingCode asks the page for a linking code for phone. See
// internal/wa-headless/capabilities/phonepair.Reader.Request for the
// measured call sequence and error shape.
func (p *PhonePairer) RequestPairingCode(ctx context.Context, txtID, phone string) (string, error) {
	eval, err := p.sessions.EvaluatorForPairing(ctx, txtID)
	if err != nil {
		return "", err
	}
	code, err := phonepair.New(p.sessions.Runner(), eval).Request(ctx, phone, phonePairLabel)
	if err != nil {
		return "", apperr.New(codePairingFailedCode, apperr.CategoryValidation, err.Error(), false, err)
	}
	return code, nil
}

var _ appport.PhonePairer = (*PhonePairer)(nil)
