// Package pairing adapta o pareamento por telefone do wa-noise para a porta
// port.PhonePairer.
package pairing

import (
	"context"

	wapairing "wa-api/internal/noise/capabilities/pairing"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
	clientpkg "wa-api/pkg/infra/noise/client"
	wasession "wa-api/pkg/infra/noise/runtime/session"
)

const (
	// pairClientDisplayName is the `Browser (OS)` label the WhatsApp server
	// validates — it answers 400 for anything outside the common set. The
	// value is the one the historical handler sent (41bc8e2^:handlers.go:733),
	// kept verbatim: it is wire contract with the server, not a preference.
	pairClientDisplayName = "Chrome (Linux)"

	// pairShowPushNotification asks the phone to surface a push notification
	// for the pairing request. Also verbatim from the historical handler.
	pairShowPushNotification = true

	// pairClientType is the platform id sent alongside the display name.
	pairClientType = wapairing.ClientChrome
)

// codePairingFailedCode is the apperr code for a pairing request the WhatsApp
// server (or the local phone-number validation) refused.
const codePairingFailedCode = "pair_phone_failed"

// PhonePairerAdapter implements appport.PhonePairer over the clientManager.
//
// It embeds SessionGuardAdapter for the same reason every other capability
// adapter does: resolving "is there a session for this txtID?" is a single
// point (runtime/session/guard.go), not a nil-check repeated per adapter.
type PhonePairerAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewPhonePairerAdapter builds the adapter from the client lookup function.
func NewPhonePairerAdapter(getClient clientpkg.Getter) *PhonePairerAdapter {
	return &PhonePairerAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// IsPaired reports whether the session is already authenticated.
func (a *PhonePairerAdapter) IsPaired(_ context.Context, txtID string) (bool, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return false, err
	}
	return client.IsLoggedIn(), nil
}

// RequestPairingCode asks the WhatsApp server for the linking code.
//
// The failure is typed as CategoryValidation because the historical contract
// answered 400 for every error out of PairPhone (41bc8e2^:handlers.go:737-741),
// and the errors that actually reach here are phone-number ones — too short,
// or national instead of international
// (internal/noise/capabilities/pairing/paircode.go:58-62). Keeping 400 is
// deliberate contract fidelity, recorded in HOUSEKEEP F152.
func (a *PhonePairerAdapter) RequestPairingCode(ctx context.Context, txtID, phone string) (string, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return "", err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, clientpkg.RequestTimeout)
	defer cancel()

	code, err := client.PairPhone(ctxWithTimeout, phone, pairShowPushNotification, pairClientType, pairClientDisplayName)
	if err != nil {
		return "", apperr.New(
			codePairingFailedCode,
			apperr.CategoryValidation,
			err.Error(),
			false,
			err,
		)
	}
	return code, nil
}

// Compile-time assertion that the adapter implements the port.
var _ appport.PhonePairer = (*PhonePairerAdapter)(nil)
