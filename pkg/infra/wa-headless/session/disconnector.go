// Package session adapts session lifecycle to the application's ports.
//
// # Why this satisfies SessionDisconnector and REFUSES SessionLogouter
//
// The two look symmetric and are not. Disconnecting drops the transport: the
// session can come back on its own. Logging out DEAUTHENTICATES — and on a page
// transport that unpairs the account, which needs a human holding the phone to
// restore.
//
// H122 measured that the operation EXISTS and works in this build. So the
// refusal here is NOT "we cannot": it is "we will not", and the difference
// matters. This is a fifth kind of divergence from the socket transport, after
// no-meaning, human-dependency, missing-datum and stale-answer: the capability
// WORKS, and exercising it costs something only a human can put back.
//
// Implementing it to satisfy the compiler would mean calling the operation that
// works — deleting a pairing nobody asked to delete. Refusing the port makes
// that impossible to reach by accident rather than by discipline.
package session

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"
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

// SessionStatus reports ownership, and says so honestly.
//
// ADR-0005 D6: intent and observed state are two questions, and a probe that
// conflates them lies. This process knows it HOLDS the session; whether the page
// is actually usable is what the liveness capability answers, and answering it
// here would mean booting a browser to reply to a status query.
//
// So connected mirrors ownership, and loggedIn is reported the same: a held
// session was restored from a paired profile. Claiming more would be inventing.
func (d *Disconnector) SessionStatus(_ context.Context, txtID string) (connected, loggedIn bool) {
	held := d.sessions.Holds(txtID)
	return held, held
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
