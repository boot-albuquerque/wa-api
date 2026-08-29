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
	"sync"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/wa-headless"

	"github.com/rs/zerolog/log"
)

// Disconnector implements appport.SessionDisconnector over a headless session.
type Disconnector struct {
	sessions *adapter.Sessions

	mu           sync.Mutex
	everIdentity map[string]bool
}

// NewDisconnector builds the adapter.
func NewDisconnector(sessions *adapter.Sessions) *Disconnector {
	return &Disconnector{sessions: sessions, everIdentity: make(map[string]bool)}
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
//
// # A session that WAS paired and lost its identity is not the same as one
// # that never paired (HOUSEKEEP F378)
//
// Both shapes make RefreshOwnIdentity fail the same way — a QR screen with
// no owner — because that is genuinely what the page shows in both cases.
// MEASURED live (2026-08-29, real headless session, phone unlinked the
// device): the page recovers ON ITS OWN to a fresh, fully working QR —
// GET /session/pair/qr answers immediately with code_age_seconds=0 — with
// nothing here to tell a caller this was a REMOTE LOGOUT rather than a
// session that simply never got scanned yet. Reporting (true, false) for
// both, as before this fix, produced "conectada, não autenticada" for an
// event wa_noise reports as fully disconnected (whatsmeow's client drops
// IsConnected() on the same real-world trigger) — the same physical action
// (unlink from the phone) read as two different states depending only on
// which engine the session happened to use.
//
// everIdentity remembers, per txtID, whether THIS ADAPTER has ever observed
// a present identity. Once it has, a later absence is read as "this session
// was connected and lost its identity" and reported as disconnected — same
// shape wa_noise already reports for the identical trigger — instead of the
// misleading "still pairing" shape a session that never paired also
// produces. In-memory, per PROCESS lifetime (not persisted): a restart
// forgets it, and the next status poll for an actually-still-logged-out
// session would read as "conectada, não autenticada" once, until the next
// RefreshOwnIdentity call — the same limitation F374/F377's per-process
// trackers already accept for the same reason (no other object here lives
// across a restart to remember it in).
func (d *Disconnector) SessionStatus(ctx context.Context, txtID string) (connected, loggedIn bool) {
	if !d.sessions.Holds(txtID) {
		d.forgetIdentity(txtID)
		return false, false
	}
	eval, err := d.sessions.Evaluator(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("session status: held but evaluator unavailable")
		return false, false
	}
	identity, err := waheadless.RefreshOwnIdentity(ctx, d.sessions.Runner(), eval, statusLabel)
	present := err == nil && identity.Present()
	connected, loggedIn, markSeen := classifyIdentity(d.hadIdentity(txtID), present)
	if markSeen {
		d.markIdentitySeen(txtID)
	}
	return connected, loggedIn
}

// classifyIdentity is the decision table SessionStatus applies, pulled out
// as a pure function so the transition F378 fixes (present now vs. present
// before) is testable without a live page. everHad is whatever
// hadIdentity(txtID) already reports; present is whether THIS read found an
// identity.
func classifyIdentity(everHad, present bool) (connected, loggedIn, markSeen bool) {
	if present {
		return true, true, true
	}
	if everHad {
		// Was paired, now isn't — WhatsApp itself ended the session
		// (typically: phone unlinked the device). Report it the way
		// wa_noise already reports the identical trigger: disconnected, not
		// "still pairing" (HOUSEKEEP F378).
		return false, false, false
	}
	// Never confirmed an identity for this txtID — a fresh, unpaired
	// session, genuinely mid-pairing.
	return true, false, false
}

func (d *Disconnector) markIdentitySeen(txtID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.everIdentity[txtID] = true
}

func (d *Disconnector) hadIdentity(txtID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.everIdentity[txtID]
}

func (d *Disconnector) forgetIdentity(txtID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.everIdentity, txtID)
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
