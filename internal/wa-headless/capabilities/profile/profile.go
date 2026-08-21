// Package profile changes this account's own display name.
//
// THE BUILD REFUSES IT ON THE LAB ACCOUNT, and that is measured rather than
// assumed. Conn.canSetMyPushname() is !getIsSMB(this), and it returned FALSE —
// so the lab account is a WhatsApp Business account. That fact is worth more
// than this capability: it is a property of the fixture every other measurement
// in this module was taken against.
//
// The guard is asked BEFORE the call, so a caller gets ErrCannotSetDisplayName
// and the reason, instead of whatever the app does when asked anyway.
//
// setPushname(name, onDone) — the second argument is a UI callback (the app
// passes one that refocuses an edit button) and is omitted here.
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Bounds. Var, not const, so tests can compress the clock.
var (
	profileBudget = 30 * time.Second
	profileTick   = 500 * time.Millisecond
)

// MaxDisplayNameBytes is this package's ceiling. WhatsApp's is smaller and the
// page refuses first; this exists so an accidental paragraph is refused here.
const MaxDisplayNameBytes = 256

var (
	// ErrProfile is the page refusing or throwing.
	ErrProfile = fmt.Errorf("profile: the page refused")
	// ErrCannotSetDisplayName is the build's own refusal. Measured false on the
	// lab account, which is a Business account.
	ErrCannotSetDisplayName = fmt.Errorf("profile: this build will not let this account change its display name (business accounts cannot)")
	// ErrEmptyDisplayName is a caller trying to clear the name.
	ErrEmptyDisplayName = fmt.Errorf("profile: empty display name")
	// ErrDisplayNameTooLong is the ceiling above.
	ErrDisplayNameTooLong = fmt.Errorf("profile: display name is longer than this package will send")
	// ErrDisplayNameUnchanged is the postcondition.
	ErrDisplayNameUnchanged = fmt.Errorf("profile: the page accepted the change and the display name did not change")
)

// NameChange is what a rename did. Lengths, not names: an account's display
// name identifies a person.
type NameChange struct {
	FromLen, ToLen int
	NoOp           bool
	Waited         time.Duration
}

func (n NameChange) String() string {
	return fmt.Sprintf("profile.NameChange(fromLen=%d toLen=%d noop=%t waited=%s)",
		n.FromLen, n.ToLen, n.NoOp, n.Waited.Round(time.Millisecond))
}

// Editor changes this account's profile.
type Editor struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds an Editor.
func New(runner *engine.Runner, eval spa.Evaluator) *Editor {
	return &Editor{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessProfile"

// SetDisplayName changes the name other people see beside this account's
// messages.
func (e *Editor) SetDisplayName(ctx context.Context, name, label string) (NameChange, error) {
	if strings.TrimSpace(name) == "" {
		return NameChange{}, ErrEmptyDisplayName
	}
	if len(name) > MaxDisplayNameBytes {
		return NameChange{}, fmt.Errorf("%w (%d bytes, ceiling %d)",
			ErrDisplayNameTooLong, len(name), MaxDisplayNameBytes)
	}
	start := time.Now()

	var kicked string
	if err := e.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return e.eval(ctx, nameScript(name), &kicked)
	}); err != nil {
		return NameChange{}, fmt.Errorf("%w: %v", ErrProfile, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		FromLen int    `json:"fromLen"`
		ToLen   int    `json:"toLen"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(profileBudget)
	for {
		var raw string
		if err := e.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return e.eval(ctx, resultScript, &raw)
		}); err != nil {
			return NameChange{}, fmt.Errorf("%w: %v", ErrProfile, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return NameChange{}, fmt.Errorf("profile: unexpected answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			if out.Stage == "settling" {
				return NameChange{}, fmt.Errorf("%w within %s", ErrDisplayNameUnchanged, profileBudget)
			}
			return NameChange{}, fmt.Errorf("%w: the page never settled within %s", ErrProfile, profileBudget)
		}
		time.Sleep(profileTick)
	}

	switch {
	case out.Why == "CANNOT_SET":
		return NameChange{}, ErrCannotSetDisplayName
	case !out.OK:
		return NameChange{}, fmt.Errorf("%w at %s (%s)", ErrProfile, out.Stage, out.Why)
	}
	return NameChange{FromLen: out.FromLen, ToLen: out.ToLen,
		NoOp: out.Already, Waited: time.Since(start)}, nil
}

func nameScript(name string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
		let stage = 'allowed';
		try {
			const want = ` + strconv.Quote(name) + `;
			const Conn = window.require('` + string(spa.ModuleConnModel) + `').Conn;
			// THE BUILD'S OWN GATE, asked first. It is !getIsSMB(this), and it
			// measured false on the lab account.
			if (Conn.canSetMyPushname && !Conn.canSetMyPushname()) {
				park({ stage, ok: false, why: 'CANNOT_SET' }); return;
			}
			const before = String(Conn.pushname || '');
			if (before === want) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true,
					fromLen: before.length, toLen: before.length });
				return;
			}

			stage = 'apply';
			const A = window.require('` + string(spa.ModuleSetPushnameConnAction) + `');
			// The second argument is a UI callback in the app; omitted here
			// because a headless session has no edit button to refocus.
			await A.setPushname(want);

			stage = 'verify';
			// Conn is parked, not its value: the await resolves before the model
			// moves (H61, measured at 696ms on a sibling capability).
			park({ stage: 'settling', ok: true, why: '', already: false,
				fromLen: before.length, want: want, conn: Conn });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		const now = String((s.conn && s.conn.pushname) || '');
		if (now === s.want) {
			return { stage: 'done', ok: true, why: '', already: false,
				fromLen: s.fromLen, toLen: now.length };
		}
		return { stage: 'settling', ok: false, why: '', fromLen: s.fromLen };
	}
	return s;
})())`
