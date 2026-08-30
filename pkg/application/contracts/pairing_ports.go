package port

import "context"

// The pairing surface: the three operations that turn "a session record
// exists" into "a session is authenticated with WhatsApp".
//
// They are declared together, and apart from SessionController, because they
// sit on the OTHER side of the same line: SessionController answers lifecycle
// questions about a session that is already up (disconnect, logout), and these
// answer how one comes up at all. PhonePairer (phone_pairer.go) is the third
// member of this family and predates the file only because CAP-26 needed it
// first.
//
// Every one of them is engine-conditioned. That is the whole reason they are
// ports rather than direct calls into an adapter: until 2026-08-27 the HTTP
// handlers behind /session/qr, /session/connect and /session/pairphone were
// wired to a hardcoded noise adapter, so a session persisted with
// engine=headless still paired over the socket. See HOUSEKEEP F273.

// PairingQRReader offers the QR code a human points a phone at.
//
// # Why this is a port and not a database read
//
// The code lives in users.qrcode, which is engine-agnostic storage, so reading
// it looks like it belongs to the user repository. It does not. The column only
// ever holds a value because ONE engine's lifecycle listener writes it there
// (pkg/bootstrap/lifecycle.go, on every "code" event of the noise QR
// channel). An engine with no such writer would serve an empty string forever
// and answer 200, which is the exact shape of failure this repository keeps
// paying for: a truthful-looking success for something that never happened.
//
// Behind the port, an engine that cannot pair by QR simply has no adapter, and
// the capability registry answers capability_not_supported instead.
type PairingQRReader interface {
	SessionGuard

	// PairingQR returns the pairing code currently on offer for txtID, or an
	// empty string when there is none right now. An empty string is not an
	// error: the QR rotates, and the window between two codes is normal.
	PairingQR(ctx context.Context, txtID string) (string, error)
}

// SessionStarter starts the transport a pairing flow needs.
//
// # Why it does NOT embed SessionGuard
//
// Every other provider-dependent port in this package embeds it, because
// nearly all of them ask "is there a session for this txtID?" before doing
// anything. This one cannot: it is what MAKES the session, so requiring one to
// already exist is a contradiction. On noise, EnsureSession fails with
// exactly the state connect is called in.
//
// The consequence is deliberate and visible: coverage_gate_test.go enumerates
// provider-dependent ports by "embeds SessionGuard", so this interface is
// invisible to that scan. Its capability mapping is registered in
// capabilityregistry.PortCoverage by hand instead
// ({"SessionStarter","StartSession"} -> domain.CapConnectSession), so
// connect_session still appears in /session/capabilities and
// /admin/capabilities rather than silently falling out of both.
type SessionStarter interface {
	// CheckOwnership reports whether THIS process may drive txtID.
	//
	// It is separate from StartSession, and synchronous, because F108: the
	// handler answered 200 {"status":"connecting"} and only then discovered
	// in a goroutine that ownership was denied — the response lied. The check
	// is idempotent; StartSession claims the same lease again and succeeds
	// because the owner is the same process.
	CheckOwnership(ctx context.Context, txtID string) error

	// StartSession begins the transport for txtID.
	//
	// It returns nothing on purpose. Connecting is fire-and-forget by
	// contract: the QR appears later, on GET /session/qr, and the historical
	// body of /session/connect is {"status":"connecting"} precisely because
	// the outcome is not known when the response is written (see
	// pkg/application/session/orchestrator.go). A port that returned an error
	// here would invite a caller to wait for one.
	StartSession(ctx context.Context, txtID, token string)
}
