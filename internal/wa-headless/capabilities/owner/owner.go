// Package owner answers the product's refreshOwner: who is this session logged
// in as?
//
// It is the stack's first real READ of the SPA, which is why the parity matrix
// puts it third — after liveness and the pid, before anything that touches
// messages. It reads, it never writes, and it never sends.
//
// WHERE THE KNOWLEDGE COMES FROM. The owner identity lives in
// spa.ModuleUserPrefsMeUser, which is where whatsapp-web.js reads it
// (Client.js:351-364 in 1.34.7). EVIDENCIA-SPA.md M3 measured that module
// directly: its getters answer EMPTY for a full 75s on an unpaired profile and
// PRESENT at T+0.01s on a paired one, and the WID object it returns carries the
// keys _serialized, server and user. None of that is guessed.
//
// PII. Unlike every other value this module returns, an Identity IS the
// account: a phone number, a LID, a display name. Two consequences are built in
// rather than left to callers — Identity implements fmt.Stringer and fmt.GoStringer
// with REDACTED output, so a %v or %+v in someone's log cannot leak it by
// accident, and nothing in this package logs.
package owner

import (
	"context"
	"encoding/json"
	"fmt"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// WID is one WhatsApp identifier, in the shape the page hands it over.
//
// The three fields are the keys M3 measured on the returned object; nothing is
// invented and nothing is parsed out of Serialized, because splitting a
// serialized identifier by hand is how a stack starts disagreeing with the
// server about who someone is.
type WID struct {
	User       string `json:"user"`
	Server     string `json:"server"`
	Serialized string `json:"_serialized"`
}

// Present reports whether this identifier was materialised at all.
func (w WID) Present() bool { return w.Serialized != "" || w.User != "" }

// String redacts. See the package comment: this value is the account.
func (w WID) String() string {
	if !w.Present() {
		return "WID(absent)"
	}
	return fmt.Sprintf("WID(present, server=%s)", w.Server)
}

// Identity is who the session is logged in as.
//
// A CONSCIOUS DIVERGENCE FROM whatsapp-web.js, recorded because CLAUDE.md
// requires divergences to be deliberate rather than accidental: wwebjs collapses
// the two identifiers into one field, `wid: getMaybeMePnUser() || getMaybeMeLidUser()`.
// This keeps them SEPARATE.
//
// The reason is this product's own history. LID and PN are different namespaces
// for the same person, the wa-api has a documented history of confusing them,
// and `pn || lid` discards WHICH one answered — so a caller holding the result
// cannot tell whether it has a phone-number identity or a LID, and the two are
// not interchangeable when talking to the server. Collapsing is cheap for a
// library whose callers only display the value; it is not cheap here.
type Identity struct {
	// PN is the phone-number identity, from getMaybeMePnUser().
	PN WID
	// LID is the LID identity, from getMaybeMeLidUser().
	LID WID
	// DisplayName is the account's own pushname, from getMaybeMeDisplayName().
	// It is PII like the rest and is redacted by String().
	//
	// MEASURED, NOT ASSUMED, AND NEVER SEEN POPULATED (2026-08-19). Against the
	// real paired profile the getter EXISTS — it is a function, not absent —
	// and returns null. Two candidates on the neighbouring module
	// (WAWebUserPrefsInfoStore.getPushname, .getMe) do not exist at all; those
	// were guesses and the measurement discarded them.
	//
	// So this field is part of the API surface and is UNVERIFIED: no caller
	// should treat a non-empty DisplayName as guaranteed, and an empty one is
	// not evidence of anything about the session. Whether it is null because
	// this account never set a pushname or because this build populates it
	// elsewhere is UNKNOWN — see TestRealSPADisplayNameShape, which is the
	// instrument that would tell the two apart the day it changes.
	DisplayName string
}

// Present reports whether the session has an owner identity at all.
//
// EITHER identifier counts, matching what whatsapp-web.js treats as "there is a
// wid". The divergence above is about not LOSING which one answered, not about
// requiring both: M3 measured both present together on a paired profile, but a
// build that materialises one before the other would make requiring both a
// false negative about the session.
func (i Identity) Present() bool { return i.PN.Present() || i.LID.Present() }

// String redacts. It reports SHAPE — which identifiers exist — never values.
func (i Identity) String() string {
	return fmt.Sprintf("Identity(pn=%v lid=%v display_name=%v)",
		i.PN.Present(), i.LID.Present(), i.DisplayName != "")
}

// GoString redacts too, so %#v is no leakier than %v.
func (i Identity) GoString() string { return i.String() }

// ErrNoOwner is a page that answered, and answered that nobody is logged in.
//
// It is distinct from a probe error on purpose: a session showing a QR is a
// working browser with no account, and the repair — pair it — has nothing to do
// with the repair for a page that failed to answer.
var ErrNoOwner = fmt.Errorf("owner: the session has no owner identity")

// readScript asks the identity module for both identifiers and the display
// name, and returns SHAPE-AND-VALUE as JSON.
//
// It reads only through spa.ModuleUserPrefsMeUser, the module the boot now
// verifies (added to spa.RequiredAtStartup with this capability), so a missing
// module fails the boot with a named cause instead of surfacing here as an
// empty identity — which would be indistinguishable from a logged-out session.
func readScript() string {
	m := string(spa.ModuleUserPrefsMeUser)
	return `JSON.stringify((() => {
		const out = { ok: false, pn: null, lid: null, display_name: "" };
		try {
			const me = window.require('` + m + `');
			if (!me) return out;
			out.ok = true;
			const wid = (fn) => {
				try {
					const v = (typeof me[fn] === 'function') ? me[fn]() : null;
					if (!v) return null;
					return { user: v.user || "", server: v.server || "", _serialized: v._serialized || "" };
				} catch (e) { return null; }
			};
			out.pn = wid('getMaybeMePnUser');
			out.lid = wid('getMaybeMeLidUser');
			try {
				const n = (typeof me.getMaybeMeDisplayName === 'function') ? me.getMaybeMeDisplayName() : "";
				out.display_name = (typeof n === 'string') ? n : "";
			} catch (e) {}
		} catch (e) { out.ok = false; }
		return out;
	})())`
}

type wireResult struct {
	OK          bool   `json:"ok"`
	PN          *WID   `json:"pn"`
	LID         *WID   `json:"lid"`
	DisplayName string `json:"display_name"`
}

// Refresh reads the owner identity from a live session.
//
// Three outcomes, kept apart because the caller's repair differs in each: an
// error means the page could not be asked and NOTHING is known; ErrNoOwner
// means the page answered and there is no account; a returned Identity means
// there is one. Folding the first two together is the mistake this module has
// paid for repeatedly (ARMADILHAS.md) — an unread probe reported as an absence
// is an instrument inventing a result.
func Refresh(ctx context.Context, runner *engine.Runner, eval spa.Evaluator, label string) (Identity, error) {
	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, label, func(ctx context.Context) error {
		return eval(ctx, readScript(), &raw)
	}); err != nil {
		return Identity{}, fmt.Errorf("owner: reading the identity module: %w", err)
	}

	var wire wireResult
	if err := json.Unmarshal([]byte(raw), &wire); err != nil {
		// The page answered with something this build does not understand.
		// Reporting that as "no owner" would blame the account for a parsing
		// problem.
		return Identity{}, fmt.Errorf("owner: unexpected answer shape: %w", err)
	}
	if !wire.OK {
		return Identity{}, fmt.Errorf("owner: %s did not resolve in the page; the boot "+
			"inventory should have caught this", spa.ModuleUserPrefsMeUser)
	}

	var id Identity
	if wire.PN != nil {
		id.PN = *wire.PN
	}
	if wire.LID != nil {
		id.LID = *wire.LID
	}
	id.DisplayName = wire.DisplayName

	if !id.Present() {
		return Identity{}, ErrNoOwner
	}
	return id, nil
}
