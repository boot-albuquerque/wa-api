package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

const (
	// codeMissingPhone marks a request without the Phone field.
	codeMissingPhone = "missing_phone"
	// codeAlreadyPaired marks a pairing request for a session that is
	// already authenticated.
	codeAlreadyPaired = "already_paired"

	// msgAlreadyPaired is the historical message of 41bc8e2^:handlers.go:731,
	// byte for byte.
	//
	// msgMissingPhone is NOT: the historical text is
	// "missing Phone in Payload", with a capital P in Payload
	// (41bc8e2^:handlers.go:722), and this one is lowercase. The divergence
	// arrived with the migration to the use case, NOT with CAP-26 — the
	// lowercase form was already in the tree at 2823a9c. It is preserved on
	// purpose: the message is public contract, integrators may match on it,
	// and "restoring fidelity" now would be a contract change outside this
	// block's scope. Recorded in HOUSEKEEP F152.
	msgMissingPhone  = "missing Phone in payload"
	msgAlreadyPaired = "already paired"
)

// PairPhoneUseCase produces the linking code for pairing by phone number —
// the alternative to the QR code.
type PairPhoneUseCase struct {
	pairer appport.PhonePairer
	logger appport.Logger
}

// NewPairPhoneUseCase builds the use case over the pairing port.
func NewPairPhoneUseCase(pp appport.PhonePairer, l appport.Logger) *PairPhoneUseCase {
	return &PairPhoneUseCase{
		pairer: pp,
		logger: l,
	}
}

// Execute validates the request and returns the linking code the user types
// on the phone.
//
// The step ORDER is load-bearing and is what CAP-26 locks in a test. The
// already-paired guard runs BEFORE RequestPairingCode, exactly as
// 41bc8e2^:handlers.go:729-741 did: asking the WhatsApp server for a code for
// a session that is already authenticated is a pointless round trip, and
// swapping the two still returns a code — so every test that only inspects
// the happy path keeps passing with the guard defeated.
func (uc *PairPhoneUseCase) Execute(ctx context.Context, txtID string, req domain.PairPhoneRequest) (*domain.PairPhoneResult, error) {
	if req.Phone == "" {
		return nil, apperr.New(codeMissingPhone, apperr.CategoryValidation, msgMissingPhone, false, nil)
	}

	if err := uc.pairer.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	paired, err := uc.pairer.IsPaired(ctx, txtID)
	if err != nil {
		uc.logger.Warn(ctx, "pair phone status check failed", "txtID", txtID, "error", err)
		return nil, err
	}
	if paired {
		uc.logger.Warn(ctx, msgAlreadyPaired, "txtID", txtID)
		return nil, apperr.New(codeAlreadyPaired, apperr.CategoryValidation, msgAlreadyPaired, false, nil)
	}

	code, err := uc.pairer.RequestPairingCode(ctx, txtID, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "pair phone request failed", "txtID", txtID, "error", err)
		return nil, err
	}

	uc.logger.Info(ctx, "pair phone code issued", "txtID", txtID)
	return &domain.PairPhoneResult{LinkingCode: code}, nil
}
