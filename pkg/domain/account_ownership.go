package domain

import "context"

// Account-identity ownership contracts (feature/engine-session-ownership).
//
// The storage and the atomic claim live in pkg/infra/db/account_ownership.go
// (POSTGRES ONLY, same as session_leases). This file holds the DOMAIN-level
// vocabulary two other worktrees plug into:
//
//   - provider-noise / provider-headless implement Fence to react to
//     losing ownership (item 7).
//   - the HTTP auth middleware uses ErrSessionSuperseded to answer a request
//     authenticated with a superseded session's token (item 6).

// AccountOwnershipStatus mirrors pkg/infra/db's status column as a typed
// domain value, so callers outside pkg/infra/db do not compare raw strings.
type AccountOwnershipStatus string

const (
	AccountOwnershipActive     AccountOwnershipStatus = "active"
	AccountOwnershipSuperseded AccountOwnershipStatus = "superseded"
)

// ErrSessionSuperseded is the STABLE error a request authenticated with a
// superseded session's token must fail with (item 6): 409, no detail about
// the session that replaced it.
//
// It deliberately carries NO fields (not even the winning session_id): the
// prompt asks for "sem vazar detalhes da nova sessão", and a struct with a
// hidden field is one refactor away from that detail leaking into a log line
// or a response body written by someone who did not re-read this comment.
// A caller who needs to know *why* logs the AccountOwnership row it already
// had in hand before returning this error — never derives it from the error
// itself.
const sessionSupersededCode = "session_superseded"

// sessionSupersededError is its own unexported type — not errors.New —
// specifically so callers compare it with errors.Is against the sentinel
// below, never by inspecting message text (F236 in HOUSEKEEP.md warned
// exactly about this: an *apperr.AppError compared by errors.New identity is
// a bug that compiles).
type sessionSupersededError struct{}

func (e *sessionSupersededError) Error() string { return sessionSupersededCode }

var ErrSessionSuperseded error = &sessionSupersededError{}

// pkg/domain deliberately does not import apperr here: apperr is a
// category-driven type meant for the HTTP boundary, and pkg/domain must stay
// importable by anything, apperr included, without a cycle risk. The HTTP
// middleware that CONSUMES ErrSessionSuperseded is expected to wrap it into
// an *apperr.AppError with apperr.CategoryConflict (409, already the
// category used for "well formed, authorized, wrong owner" — see
// apperr/codes.go) at the boundary.

// Fencer is the contract a runtime provider (noise, headless) must
// satisfy so the ownership mechanism can shut down a superseded session
// WITHOUT performing a destructive upstream logout.
//
// # Why this exists here and is not implemented here
//
// This worktree (engine-session-ownership) owns deciding WHO the current
// owner is and WHEN a session has been superseded. It does not own how
// noise's socket or headless's browser session actually tears down —
// those live in the provider worktrees (provider-noise,
// provider-headless), and each has different physical resources to
// release. Fence is the seam: this package calls it, the provider
// implements it.
//
// # The semantics an implementation MUST follow
//
//  1. Fence must stop the runtime from producing further side effects
//     (sending messages, ACKing events, renewing anything) for sessionID.
//     It is NOT a request the session can refuse or delay indefinitely —
//     the caller has already decided sessionID lost ownership.
//  2. Fence must NOT perform a WhatsApp logout, revoke pairing, or destroy
//     credentials shared with the new owner. The new owner may be about to
//     reuse the SAME underlying device pairing; a destructive logout here
//     would take the new owner down with the old one. "Fenced" means "this
//     process stops touching it", not "this account is gone".
//  3. Fence must be safe to call when the session is already stopped or
//     was never fully started (e.g. superseded mid-pairing) — a no-op in
//     that case, not an error.
//  4. Fence must report failure distinctly from success (see the item-8
//     contract below): the caller records old_session_cleanup_failed and
//     keeps the NEW owner as owner regardless of what Fence returns. A
//     Fence failure must never cause a rollback of ownership — see item 8.
//
// ctx carries a deadline the caller controls; Fence must respect it rather
// than blocking past it, since a hung Fence call must not hang the claim
// path that is calling it (typically out-of-band, after the claim already
// committed — see the note on FenceOutcome below).
type Fencer interface {
	Fence(ctx context.Context, sessionID string) error
}

// FenceOutcome is what a caller of Fence records when it fails — item 8:
// "cleanup failure não ressuscita o antigo". There is deliberately no
// "retry" or "rollback" field: the new owner is ALREADY the owner (the claim
// committed to the database before Fence is ever called — see
// pkg/infra/db.AccountOwnershipRepository.ClaimAccountIdentity, which has no
// dependency on Fence succeeding), and nothing in this mechanism reverses
// that. A failed Fence is a fact to log and alert on, not a decision point.
type FenceOutcome struct {
	SessionID string
	Engine    Engine
	Err       error
}

// LogCode is the constant a caller writes to structured logs when Fence
// fails, per item 8's instruction to "registre `old_session_cleanup_failed`
// (log estruturado), sem rollback silencioso". Named here, not inlined at
// call sites, so every provider worktree and every future call site logs
// the SAME code (CLAUDE.md: zero string literal solta).
const FenceFailureLogCode = "old_session_cleanup_failed"
