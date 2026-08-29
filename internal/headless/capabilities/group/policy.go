package group

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// Group policies — who may send, who may edit the group's information, and
// whether joining needs approval.
//
// THE NAMES WERE ENUMERATED, NOT GUESSED, and the guesses would have been wrong:
// `locked` and `announce` read like the right words and are both refused. What
// the page accepts is `restrict`, `announcement` and `membership_approval_mode`.
//
// The enumeration used the app's own refusal as an oracle and passed each
// candidate the group's CURRENT value, so a valid name was a no-op. Probing
// without that would have flipped the lab group's policies one candidate at a
// time.
//
// VERIFIED IS TRUE, AND IT USED TO BE FALSE — the correction matters more than
// the capability. This file assumed H58's finding about group metadata applied
// here, and the classifier "confirmed" it. The confirmation was worthless: its
// control wrote the value the group already had, and a no-op cannot move a
// reader.
//
// Measured properly, with a real flip: the policy becomes visible IN THIS
// SESSION after about ONE SECOND. So the change is observable and the
// postcondition is real.
//
// H58 still stands for PARTICIPANTS — that was a real change polled for ninety
// seconds — and the lesson is that "group metadata is stale" was too broad a
// story built from one measurement.

// Policy names one of the group's settings.
type Policy string

const (
	// PolicyMessagesAdminsOnly is `announcement` — only admins may send.
	PolicyMessagesAdminsOnly Policy = "announcement"
	// PolicyInfoAdminsOnly is `restrict` — only admins may edit subject,
	// description and picture.
	PolicyInfoAdminsOnly Policy = "restrict"
	// PolicyJoinNeedsApproval is `membership_approval_mode`.
	PolicyJoinNeedsApproval Policy = "membership_approval_mode"
)

// known is the set this package will send. A Policy is a string type, so a
// caller can spell one that the page refuses; refusing here names the mistake
// instead of letting a TypeError from the page's switch do it.
var known = map[Policy]bool{
	PolicyMessagesAdminsOnly: true,
	PolicyInfoAdminsOnly:     true,
	PolicyJoinNeedsApproval:  true,
}

var (
	// ErrPolicy is the page refusing or throwing.
	ErrPolicy = fmt.Errorf("group: the page refused the policy change")
	// ErrUnknownPolicy is a name this package will not send.
	ErrUnknownPolicy = fmt.Errorf("group: unknown group policy")
	// ErrCannotSetPolicy is the group's own canSetGroupProperty saying no.
	ErrCannotSetPolicy = fmt.Errorf("group: this account cannot change this group's policies")
	// ErrPolicyUnchanged is the postcondition: the call returned and the
	// group's metadata still reads the old value.
	ErrPolicyUnchanged = fmt.Errorf("group: the page accepted the policy change and the metadata did not move")
)

// PolicyChange is what a policy change did.
type PolicyChange struct {
	Policy Policy
	// Wanted is the state asked for.
	Wanted bool
	// NoOp is true when the group already had it.
	NoOp bool
	// Verified says the metadata was READ BACK carrying the new value. It is
	// true for a real change here, unlike Membership.Verified — the difference
	// is measured, not assumed, and correcting it was H85.
	Verified bool
	Waited   time.Duration
}

func (p PolicyChange) String() string {
	return fmt.Sprintf("group.PolicyChange(policy=%s wanted=%t noop=%t verified=%t waited=%s)",
		p.Policy, p.Wanted, p.NoOp, p.Verified, p.Waited.Round(time.Millisecond))
}

const policyStateKey = "__headlessGroupPolicy"

// SetPolicy turns one of the group's settings on or off.
func (m *Manager) SetPolicy(ctx context.Context, groupJID string, p Policy, on bool, label string) (PolicyChange, error) {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return PolicyChange{}, ErrNotGroup
	}
	if !known[p] {
		return PolicyChange{}, fmt.Errorf("%w: %q (accepted: announcement, restrict, membership_approval_mode)",
			ErrUnknownPolicy, string(p))
	}
	start := time.Now()

	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, policyScript(groupJID, p, on), &kicked)
	}); err != nil {
		return PolicyChange{}, fmt.Errorf("%w: %v", ErrPolicy, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(createBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return m.eval(ctx, policyResultScript, &raw)
		}); err != nil {
			return PolicyChange{}, fmt.Errorf("%w: %v", ErrPolicy, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return PolicyChange{}, fmt.Errorf("group: unexpected policy answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			// A settling stage that ran out of budget is a policy that did not
			// take, not a page that hung.
			if out.Stage == "settling" {
				return PolicyChange{}, fmt.Errorf("%w within %s", ErrPolicyUnchanged, createBudget)
			}
			return PolicyChange{}, fmt.Errorf("%w: the page never settled within %s", ErrPolicy, createBudget)
		}
		time.Sleep(createTick)
	}

	switch {
	case out.Why == "NO_CHAT" || out.Why == "NO_METADATA":
		return PolicyChange{}, fmt.Errorf("%w (%s)", ErrNotGroup, out.Why)
	case out.Why == "CANNOT_SET":
		return PolicyChange{}, ErrCannotSetPolicy
	case !out.OK:
		return PolicyChange{}, fmt.Errorf("%w at %s (%s)", ErrPolicy, out.Stage, out.Why)
	}
	return PolicyChange{Policy: p, Wanted: on, NoOp: out.Already,
		Verified: true, Waited: time.Since(start)}, nil
}

// PolicyOf reports what this session sees for one of the group's settings.
//
// IT IS CORRECT AFTER A CHANGE THIS SESSION MADE, and this doc used to say the
// opposite. The correction is worth keeping visible rather than quietly
// rewritten:
//
// The old text claimed policies went stale in the session that changed them,
// "the same as Count — which is what makes cross-session the only honest
// proof". That came from H58, which measured PARTICIPANT changes staying
// invisible for ninety seconds and generalised the finding to everything
// living on the group metadata. H85 measured policies specifically and found
// them visible in about ONE SECOND, with eight bus events to go with it; the
// classification that said otherwise had come from a control that wrote the
// value the group already held, and a no-op cannot move a reader.
//
// H90 contradicted the old text again from a different direction: a set-then-
// read in a single session reports true -> false, in that session, every time.
//
// Count IS still the stale case, and merging the two was the original mistake.
// SetPolicy therefore verifies in-session and returns Verified: true, which
// would have been unreachable if this doc had been right.
func (m *Manager) PolicyOf(ctx context.Context, groupJID string, p Policy, label string) (bool, error) {
	if !strings.HasSuffix(strings.TrimSpace(groupJID), "@g.us") {
		return false, ErrNotGroup
	}
	if !known[p] {
		return false, fmt.Errorf("%w: %q", ErrUnknownPolicy, string(p))
	}
	var raw string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/policy-read", func(ctx context.Context) error {
		return m.eval(ctx, policyReadScript(groupJID, p), &raw)
	}); err != nil {
		return false, fmt.Errorf("%w: %v", ErrPolicy, err)
	}
	switch strings.TrimSpace(raw) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("%w: the group's metadata is not loaded", ErrNotGroup)
}

// field maps a policy to the metadata field that reflects it. They are NOT the
// same strings as the property names — `announcement` is set and `announce` is
// read — which is exactly the kind of asymmetry this codebase keeps producing.
var field = map[Policy]string{
	PolicyMessagesAdminsOnly: "announce",
	PolicyInfoAdminsOnly:     "restrict",
	PolicyJoinNeedsApproval:  "membershipApprovalMode",
}

func policyScript(groupJID string, p Policy, on bool) string {
	value := "0"
	if on {
		value = "1"
	}
	return `JSON.stringify((() => {
		window[` + strconv.Quote(policyStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(policyStateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const W = window.require('` + string(spa.ModuleWidFactory) + `');
			const C = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = C.get(W.createWid(` + strconv.Quote(groupJID) + `));
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }
			const md = chat.groupMetadata;
			if (!md) { park({ stage, ok: false, why: 'NO_METADATA' }); return; }

			// THE GROUP'S OWN GATE, asked before the call. It is a METHOD, like
			// iAmAdmin was (H57) — the lesson that cost four blind attempts.
			if (typeof md.canSetGroupProperty === 'function' && !md.canSetGroupProperty()) {
				park({ stage, ok: false, why: 'CANNOT_SET' }); return;
			}

			const want = ` + strconv.FormatBool(on) + `;
			const current = !!md[` + strconv.Quote(field[p]) + `];
			if (current === want) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true }); return;
			}

			stage = 'apply';
			const A = window.require('` + string(spa.ModuleSetPropertyGroupAction) + `');
			// THREE POSITIONAL: the chat MODEL, the property NAME, and 1 or 0 —
			// the app's own switch compares the value to 1.
			await A.setGroupProperty(chat, ` + strconv.Quote(string(p)) + `, ` + value + `);

			// THE METADATA IS PARKED and Go polls it: measured at ~1s, and the
			// await settles well before that (H61).
			park({ stage: 'settling', ok: true, why: '', already: false,
				want: want, md: md, field: ` + strconv.Quote(field[p]) + ` });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

func policyReadScript(groupJID string, p Policy) string {
	return `(() => {
		try {
			const W = window.require('` + string(spa.ModuleWidFactory) + `');
			const C = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = C.get(W.createWid(` + strconv.Quote(groupJID) + `));
			const md = chat && chat.groupMetadata;
			if (!md) { return 'no'; }
			return String(!!md[` + strconv.Quote(field[p]) + `]);
		} catch (e) { return 'no'; }
	})()`
}

// policyResultScript re-reads the parked metadata each round rather than
// trusting a value captured before the change landed.
const policyResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + policyStateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		const now = !!(s.md && s.md[s.field]);
		if (now === s.want) {
			return { stage: 'done', ok: true, why: '', already: false };
		}
		return { stage: 'settling', ok: false, why: '' };
	}
	return s;
})())`
