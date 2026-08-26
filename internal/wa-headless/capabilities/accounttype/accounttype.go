// Package accounttype answers "is this session's own account a WhatsApp
// Business account", reusing the ONE signal this codebase has already
// measured live for that question.
//
// # WHERE THE KNOWLEDGE COMES FROM
//
// profile.go (this session's display-name capability) measured, against the
// real paired lab profile, that WAWebConnModel's `Conn.canSetMyPushname()` is
// `!getIsSMB(this)` and that it returned FALSE on that account — which is
// therefore a WhatsApp Business account (HOUSEKEEP.md, "O achado que vale além
// destas capacidades: a conta de laboratório é BUSINESS"). SMB is WhatsApp's
// own name for the Business tier ("small and medium business"), so
// getIsSMB(this) IS the account-type bit, read directly off the live
// connection model — not inferred from an absent error, a missing capability,
// or a heuristic over other fields.
//
// canSetMyPushname is called instead of a hypothetical getIsSMB directly
// because it is the getter this codebase has ALREADY proven resolves and
// answers correctly on the paired lab profile (profile.go); a fresh, unproven
// getter name would be exactly the "invented classification" CLAUDE.md warns
// against.
//
// # WHAT THIS PACKAGE DOES NOT CLAIM
//
// Conn.canSetMyPushname existing and answering a bool is confirmed. Whether
// getIsSMB is ALSO true for other account tiers this project has never seen
// (e.g. a WABA-linked number that is not the classic Business app) is
// unmeasured — this package reports what the field says, and a caller that
// needs a stronger guarantee should treat "business" as "this account cannot
// freely rename itself" rather than a specific commercial tier.
package accounttype

import (
	"context"
	"encoding/json"
	"fmt"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Kind is this package's own answer, kept separate from any application-level
// enum for the same reason owner.Identity is: internal/ does not import pkg/.
type Kind int

const (
	// KindUnknown means the page could not be asked, or answered in a shape
	// this build does not understand. Never paired with a business/personal
	// guess.
	KindUnknown Kind = iota
	KindPersonal
	KindBusiness
)

// wireResult is the page's answer.
type wireResult struct {
	// OK is false when the module or the getter itself is missing — a build
	// difference, not an account property.
	OK bool `json:"ok"`
	// CanSetMyPushname is Conn.canSetMyPushname()'s own answer. IsBusiness is
	// its negation: !getIsSMB(this) === canSetMyPushname.
	CanSetMyPushname bool `json:"can_set_my_pushname"`
}

func readScript() string {
	m := string(spa.ModuleConnModel)
	return `JSON.stringify((() => {
		const out = { ok: false, can_set_my_pushname: false };
		try {
			const Conn = window.require('` + m + `').Conn;
			if (!Conn || typeof Conn.canSetMyPushname !== 'function') { return out; }
			out.ok = true;
			out.can_set_my_pushname = !!Conn.canSetMyPushname();
		} catch (e) { out.ok = false; }
		return out;
	})())`
}

// Detect asks the live page whether the connected account is Business.
//
// An error means the page could not be asked at all — network/tab failure, or
// the getter itself is absent from this build (a build difference, not an
// account property). It is returned distinctly from KindUnknown-with-nil-error
// so a caller can tell "we could not check" apart from "we checked and this
// build does not carry the signal" — mirroring owner.Refresh's three-outcome
// shape (ARMADILHAS.md: an unread probe reported as an absence is an
// instrument inventing a result).
func Detect(ctx context.Context, runner *engine.Runner, eval spa.Evaluator, label string) (Kind, error) {
	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return eval(ctx, readScript(), &raw)
	}); err != nil {
		return KindUnknown, fmt.Errorf("accounttype: reading the connection model: %w", err)
	}

	var wire wireResult
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		return KindUnknown, fmt.Errorf("accounttype: unexpected answer shape: %w", err)
	}
	if !wire.OK {
		return KindUnknown, fmt.Errorf(
			"accounttype: %s.Conn.canSetMyPushname did not resolve in the page", spa.ModuleConnModel)
	}

	if wire.CanSetMyPushname {
		return KindPersonal, nil
	}
	return KindBusiness, nil
}
