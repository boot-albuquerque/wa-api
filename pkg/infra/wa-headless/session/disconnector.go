// Package session adapts session lifecycle to the application's ports.
//
// # Why this satisfies SessionLogouter now (HOUSEKEEP F381, reopening H122)
//
// Disconnecting drops the transport: the session can come back on its own.
// Logging out DEAUTHENTICATES — and on a page transport that unpairs the
// account, which needs a human holding the phone to restore. H122
// (internal/wa-headless/HOUSEKEEP.md) measured that Socket.logout EXISTS
// and is a function, but refused to CALL it: an untested call that unpairs
// a real account was not something to satisfy the compiler with, on policy
// grounds — not a technical block.
//
// That call was finally MEASURED (F381, TestProbeSocketLogout,
// internal/wa-headless/probe_logout_test.go), against a genuinely paired,
// disposable profile re-paired specifically for this: Socket.logout()
// returns without throwing, and within ~18s the page settles from
// CONNECTED to UNPAIRED with a fresh, working QR showing — the same
// clean "logged out, ready to re-pair" state a phone-initiated logout
// produces, never a crash or a stuck page. See
// internal/wa-headless/capabilities/logout for the call itself and where
// its shape comes from (whatsapp-web.js, measured, not guessed).
package session

import (
	"context"
	"sync"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/internal/wa-headless/capabilities/logout"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
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

// Logout deauthenticates the session — see this package's own doc comment
// (F381, reopening H122) for what was measured and where the call comes
// from.
//
// Two refusals mirror wa_noise's own SessionGuardAdapter.Logout
// (pkg/infra/wa-noise/runtime/session/guard.go) exactly — same codes, same
// categories, same reasoning:
//
//   - Evaluator unreachable (page cannot be reached at all) →
//     apperr.CodeSessionNotConnected, 409. This is the code
//     LogoutUseCase's own Execute checks for to call detacher.Detach even
//     on failure (F80) — a session whose transport is gone needs its LOCAL
//     state to stop lying, regardless of engine.
//   - Reachable but no owner identity (never paired, or already logged out)
//     → apperr.CodeSessionNotPaired, 409: there is no device to log out.
//
// Only once both pass does this call Socket.logout() for real.
func (d *Disconnector) Logout(ctx context.Context, txtID string) error {
	eval, err := d.sessions.Evaluator(ctx, txtID)
	if err != nil {
		return apperr.New(
			apperr.CodeSessionNotConnected,
			apperr.CategoryConflict,
			"session has no live connection; call /session/connect before logging out",
			false,
			err,
		)
	}
	identity, err := waheadless.RefreshOwnIdentity(ctx, d.sessions.Runner(), eval, statusLabel)
	if err != nil || !identity.Present() {
		return apperr.New(
			apperr.CodeSessionNotPaired,
			apperr.CategoryConflict,
			"session has a live connection but was never paired; there is no device to log out",
			false,
			err,
		)
	}
	return logout.Do(ctx, d.sessions.Runner(), eval, statusLabel)
}

// Compile-time proof: this adapter satisfies the full controller, not only
// the disconnecting half — SessionLogouter joined it in F381.
var _ appport.SessionController = (*Disconnector)(nil)
