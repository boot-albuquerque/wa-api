// Package accounttype adapts appport.AccountTypeDetector to a headless
// session, using the signal in internal/headless/capabilities/accounttype:
// WAWebConnModel's Conn.canSetMyPushname(), which is !getIsSMB(this) and was
// measured live on the lab account (HOUSEKEEP.md).
//
// # THE REVALIDATE-ON-CONNECT HOOK (item 34) IS DOCUMENTED, NOT WIRED, HERE
//
// pkg/bootstrap/eventhandler_session.go wires this same idea for noise
// inside handleConnected, because that transport dispatches a typed
// events.Connected the moment a session (re)establishes. headless has no
// equivalent lifecycle event today — pkg/infra/headless/sessions.go's
// Sessions type is asked for an Evaluator on demand per capability call, and
// nothing in this worktree's scope owns "a headless session just became
// live". Wiring a call to Detect from here would be guessing at a call site
// this worktree did not measure.
//
// The gancho is this: whichever code ends up owning that lifecycle moment
// for headless (most likely wherever pkg/infra/headless/sessions.go's
// boot path first confirms the SPA reached a paired state) should call
// Detect(ctx, txtID) right there and persist the result with
// pkg/infra/db.SetUserAccountType, mirroring handleConnected's fire-and-forget
// pattern — a usync-shaped wait (there it is literal usync; here it is a page
// evaluate) must not block whatever marks the session connected, per
// CLAUDE.md's pool invariant.
package accounttype

import (
	"context"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/headless"

	"github.com/rs/zerolog/log"
)

const detectLabel = "adapter/account-type"

// Detector implements appport.AccountTypeDetector over a headless session.
type Detector struct {
	sessions *adapter.Sessions
}

// NewDetector builds the adapter.
func NewDetector(sessions *adapter.Sessions) *Detector {
	return &Detector{sessions: sessions}
}

// EnsureSession reports whether this process can serve txtID, without booting.
func (d *Detector) EnsureSession(ctx context.Context, txtID string) error {
	return d.sessions.EnsureSession(ctx, txtID)
}

// Detect asks the live page for Conn.canSetMyPushname() and maps it to
// domain.AccountType. Any failure to read the page — tab gone, getter absent
// from this build — becomes AccountTypeUnknown with the error propagated;
// never a guessed classification.
func (d *Detector) Detect(ctx context.Context, txtID string) (domain.AccountType, error) {
	eval, err := d.sessions.Evaluator(ctx, txtID)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("account type detection: session evaluator unavailable")
		return domain.AccountTypeUnknown, err
	}
	runner := d.sessions.Runner()

	kind, err := headless.DetectAccountKind(ctx, runner, eval, detectLabel)
	if err != nil {
		log.Warn().Err(err).Str("txt_id", txtID).
			Msg("account type detection: page read failed")
		return domain.AccountTypeUnknown, err
	}
	switch kind {
	case headless.AccountKindBusiness:
		return domain.AccountTypeBusiness, nil
	case headless.AccountKindPersonal:
		return domain.AccountTypePersonal, nil
	default:
		return domain.AccountTypeUnknown, nil
	}
}

var _ appport.AccountTypeDetector = (*Detector)(nil)
