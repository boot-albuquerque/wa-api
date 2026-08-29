// Package session adapts session lifecycle to the application's ports.
//
// # Why this satisfies SessionDisconnector, and SessionLogouter is still open
//
// The two look symmetric and are not. Disconnecting drops the transport: the
// session can come back on its own. Logging out DEAUTHENTICATES — and on a page
// transport that unpairs the account, which needs a human holding the phone to
// restore.
//
// H122 (internal/wa-headless/HOUSEKEEP.md) measured that Socket.logout EXISTS
// and works in this build, but never called it — only presence was probed. This
// package's own earlier revision refused SessionLogouter on policy grounds
// (an untested call that unpairs a real account is not something to satisfy
// the compiler with). That policy decision has since been revisited on request
// (same risk wa_noise's own Logout already carries), but Logout is NOT
// implemented here yet: the JS invocation sequence for Socket.logout is
// unmeasured, and this repository's own rule is measurement before design —
// see internal/wa-headless/HOUSEKEEP.md and CLAUDE.md's anti-regression policy.
// Implementing it against a guess would risk deauthenticating a real account
// on a call shape nobody verified. See the paridade plan (session dispatch /
// pairing work) for the pending measurement step against a disposable
// .lab/ profile.
package session

import (
	"context"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"

	"github.com/rs/zerolog/log"
)

// Disconnector implements appport.SessionDisconnector over a headless session.
type Disconnector struct {
	sessions *adapter.Sessions
}

// NewDisconnector builds the adapter.
func NewDisconnector(sessions *adapter.Sessions) *Disconnector {
	return &Disconnector{sessions: sessions}
}

// EnsureSession reports whether this process can serve txtID, without booting.
func (d *Disconnector) EnsureSession(ctx context.Context, txtID string) error {
	return d.sessions.EnsureSession(ctx, txtID)
}

// statusLabel names this adapter's calls in the runner's operation log
// (engine.Runner), matching the convention every capabilities/* package
// uses for its own label constant.
const statusLabel = "adapter/session-status"

// SessionStatus reports ownership and, when a session is held, asks the page
// who it is logged in as.
//
// ADR-0005 D6: intent and observed state are two questions, and a probe that
// conflates them lies.
//
//   - connected: NOT held → false, without booting anything (GET /session/status
//     must never boot a browser just to answer a poll). Held → the
//     Evaluator resolved without error, i.e. this process could reach the
//     page at all. This is a conservative signal, not full liveness: it does
//     not yet distinguish "page slow" from "page crashed" the way
//     internal/wa-headless/capabilities/liveness does — that needs a
//     process-alive source this adapter does not have plumbed to it yet.
//   - loggedIn: only asked when connected. waheadless.RefreshOwnIdentity
//     (internal/wa-headless/capabilities/owner) distinguishes a probe error
//     (nothing known, so loggedIn stays false) from owner.ErrNoOwner (page
//     answered, showing a QR — genuinely not logged in) from a real Identity
//     (paired).
func (d *Disconnector) SessionStatus(ctx context.Context, txtID string) (connected, loggedIn bool) {
	if !d.sessions.Holds(txtID) {
		return false, false
	}
	eval, err := d.sessions.Evaluator(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("session status: held but evaluator unavailable")
		return false, false
	}
	identity, err := waheadless.RefreshOwnIdentity(ctx, d.sessions.Runner(), eval, statusLabel)
	if err != nil {
		// Includes owner.ErrNoOwner (paired-not-yet / showing QR) and any
		// read failure alike: both mean "connected, not confirmed logged
		// in" for this port's purposes.
		return true, false
	}
	return true, identity.Present()
}

// Disconnect drops the session's transport and frees its slot.
//
// The stop's FORM is discarded here because the port promises only an error —
// but it is not lost: Release records it, and a dirty stop still frees the slot,
// because a holder that failed to go down cleanly is gone as far as this process
// is concerned.
func (d *Disconnector) Disconnect(ctx context.Context, txtID string) error {
	_, err := d.sessions.Release(ctx, txtID)
	return err
}

// Compile-time proof: this adapter satisfies the disconnecting half, and ONLY
// it. The line that is absent is the point — see the package doc.
var _ appport.SessionDisconnector = (*Disconnector)(nil)
