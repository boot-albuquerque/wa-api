package port

import "context"

// PhonePairer is the phone-number pairing capability: the alternative to the
// QR code for callers who cannot point a camera at a screen.
//
// It is a capability port and not part of SessionController on purpose. The
// controller answers lifecycle questions about a session that ALREADY exists
// (logout, disconnect); pairing is what produces that session, and its only
// output — the linking code the user types on the phone — has no counterpart
// in any of the controller's methods.
//
// Before CAP-26 the use case behind POST /session/pairphone consumed plain
// SessionGuard, so the only thing it could do was assert that a session
// existed; it then answered 200 with an EMPTY LinkingCode (F152). The port
// exists so the use case has something to ask for the code.
type PhonePairer interface {
	SessionGuard

	// IsPaired reports whether the session is already authenticated with
	// WhatsApp. Asking for a pairing code for an authenticated session is
	// meaningless, and the historical contract rejected it — see
	// PairPhoneUseCase for why the guard must run BEFORE RequestPairingCode.
	IsPaired(ctx context.Context, txtID string) (bool, error)

	// RequestPairingCode asks the WhatsApp server for a linking code for
	// phone, an international number in plain digits. The returned string is
	// the code the user types on the phone; it is never empty on success.
	RequestPairingCode(ctx context.Context, txtID, phone string) (string, error)
}
