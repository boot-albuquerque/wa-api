// Package mute silences a chat's notifications for a while, and restores them.
//
// IT DRIVES THE MUTE MODEL, NOT THE BRIDGE. The module enumeration nominated
// WAWebChatMuteBridge, and the bundle showed it being called with an object
// whose key is `$MuteImpl3`. The model's own key list settles what that is:
// `$MuteImpl$p_4/5/6` are minifier artefacts of private methods. The model's
// mute/unmute are synchronous and fully legible, so that is the surface used.
//
// THE ONE ARGUMENT THAT DECIDES EVERYTHING is sendDevice. Read from the body:
//
//	if (sendDevice === true) { ...reach the bridge... }
//
// Omit it and the mute is LOCAL ONLY — and every postcondition still passes,
// because the local model is exactly what a postcondition reads. It is a silent
// half-success built into the API, and it is the reason this package asserts the
// argument in a test rather than trusting the call site.
//
// EXPIRATION IS EPOCH SECONDS. The model rejects a non-number with ActionError
// and logs "wrong units?" above 2e9. The page's own calculateMuteExpiration
// makes the value from hours, including the sentinel for "always"; the result is
// REPORTED so the caller sees the number the page chose.
package mute

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Bounds. Var, not const, so tests can compress the clock.
var (
	muteBudget = 30 * time.Second
	muteTick   = 500 * time.Millisecond
)

// Always asks for a mute with no end. It is a named constant rather than a bare
// number because the page turns it into a sentinel, and a caller writing an
// arbitrary huge hour count would get an ordinary expiry far in the future
// instead — a different thing that looks the same.
const Always = -1

var (
	// ErrMute is the page refusing or throwing.
	ErrMute = fmt.Errorf("mute: the page refused")
	// ErrNoChat is a jid with no loaded chat.
	ErrNoChat = fmt.Errorf("mute: no such chat is loaded")
	// ErrCannotMute is the page's own canMute saying no — this account's own
	// chat, or a group it is not a member of.
	ErrCannotMute = fmt.Errorf("mute: this build says this chat cannot be muted")
	// ErrBadDuration is a duration this package will not send.
	ErrBadDuration = fmt.Errorf("mute: duration must be a positive number of hours, or mute.Always")
	// ErrExpirationUnchanged is the postcondition.
	ErrExpirationUnchanged = fmt.Errorf("mute: the page accepted the change and the expiration did not move")
)

// Result is what a mute or unmute did.
type Result struct {
	// Before and After are the model's expiration, in epoch seconds. Zero means
	// not muted. They are reported rather than reduced to a boolean because the
	// page's sentinel for "always" is a number a caller may want to recognise.
	Before, After int64
	// AlreadyInState is true when there was nothing to do.
	AlreadyInState bool
	// Waited is how long the page took to reflect it.
	Waited time.Duration
}

// Changed reports whether the expiration moved.
func (r Result) Changed() bool { return r.Before != r.After }

// Muted reports whether the chat ends up silenced.
func (r Result) Muted() bool { return r.After != 0 }

func (r Result) String() string {
	return fmt.Sprintf("mute.Result(before=%d after=%d muted=%t already=%t waited=%s)",
		r.Before, r.After, r.Muted(), r.AlreadyInState, r.Waited.Round(time.Millisecond))
}

// Muter silences chats on one session.
type Muter struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Muter.
func New(runner *engine.Runner, eval spa.Evaluator) *Muter { return &Muter{runner: runner, eval: eval} }

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other, and each polled it
// until it stopped saying "pending" — so one could take the other's answer. The
// nonce comes from Go: a page-side Math.random or Date.now would put a decision
// and a clock where invariant 6 forbids them.
const stateKeyPrefix = "__waHeadlessMute"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// For silences a chat for the given number of hours, or forever with Always.
func (m *Muter) For(ctx context.Context, chatJID string, hours int, label string) (Result, error) {
	if hours != Always && hours <= 0 {
		return Result{}, fmt.Errorf("%w (got %d)", ErrBadDuration, hours)
	}
	return m.set(ctx, chatJID, hours, label)
}

// Off restores notifications.
func (m *Muter) Off(ctx context.Context, chatJID, label string) (Result, error) {
	return m.set(ctx, chatJID, 0, label)
}

func (m *Muter) set(ctx context.Context, chatJID string, hours int, label string) (Result, error) {
	if strings.TrimSpace(chatJID) == "" {
		return Result{}, ErrNoChat
	}
	start := time.Now()

	key := nextStateKey()

	var kicked string
	if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return m.eval(ctx, muteScript(chatJID, hours, key), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrMute, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Before  int64  `json:"before"`
		After   int64  `json:"after"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(muteBudget)
	for {
		var raw string
		if err := m.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return m.eval(ctx, resultScript(key), &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrMute, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("mute: unexpected answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			// Same distinction the star capability had to learn: a settling
			// stage that ran out of budget is a change that did not take, not a
			// page that hung.
			if out.Stage == "settling" {
				return Result{}, fmt.Errorf("%w within %s (before=%d after=%d)",
					ErrExpirationUnchanged, muteBudget, out.Before, out.After)
			}
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrMute, muteBudget)
		}
		time.Sleep(muteTick)
	}

	switch {
	case out.Why == "NO_CHAT" || out.Why == "NO_MUTE_MODEL":
		return Result{}, fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
	case out.Why == "CANNOT_MUTE":
		return Result{}, ErrCannotMute
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrMute, out.Stage, out.Why)
	}

	res := Result{Before: out.Before, After: out.After,
		AlreadyInState: out.Already, Waited: time.Since(start)}
	if !out.Already && !res.Changed() {
		return Result{}, fmt.Errorf("%w (before=%d after=%d)", ErrExpirationUnchanged, out.Before, out.After)
	}
	return res, nil
}

func muteScript(chatJID string, hours int, key string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(key) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(key) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const hours = ` + strconv.Itoa(hours) + `;
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(chatJID) + `);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			const MU = window.require('` + string(spa.ModuleMuteUtils) + `');
			if (MU.canMute && !MU.canMute(chat)) {
				park({ stage, ok: false, why: 'CANNOT_MUTE' }); return;
			}

			const MC = window.require('` + string(spa.ModuleMuteCollection) + `').MuteCollection;
			const model = MC.get(chat.id);
			if (!model) { park({ stage, ok: false, why: 'NO_MUTE_MODEL' }); return; }

			const before = Number(model.expiration || 0);

			// THE PAGE'S OWN CONVERTER, including its sentinel for "always".
			// Reimplementing this in Go would mean reimplementing the sentinel,
			// and the value it picks is reported back so the caller sees it.
			let wanted = 0;
			if (hours !== 0) {
				const E = window.require('` + string(spa.ModuleMuteExpirations) + `');
				wanted = Number(E.calculateMuteExpiration(
					hours === ` + strconv.Itoa(Always) + ` ? Number.POSITIVE_INFINITY : hours));
			}
			if (before === wanted) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true,
					before: before, after: before });
				return;
			}

			stage = 'apply';
			// sendDevice: true IS THE WHOLE POINT. The model's body only
			// reaches the bridge when it is exactly true; without it the change
			// is local and every postcondition below still passes.
			// showToast: false because a headless session has nobody to toast.
			if (hours === 0) { await model.unmute({ sendDevice: true, showToast: false }); }
			else { await model.mute({ expiration: wanted, sendDevice: true, showToast: false }); }

			stage = 'verify';
			// The MODEL is parked and Go polls it: the await resolves before the
			// field moves, measured at 696ms on the sibling capability (H61).
			park({ stage: 'settling', ok: true, why: '', already: false,
				before: before, want: wanted, model: model });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

// resultScript(key) reads one number off the parked model rather than serialising it.
func resultScript(key string) string {
	return `JSON.stringify((() => {
	const s = window[` + strconv.Quote(key) + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		const now = Number((s.model && s.model.expiration) || 0);
		if (now === s.want) {
			return { stage: 'done', ok: true, why: '', already: false,
				before: s.before, after: now };
		}
		return { stage: 'settling', ok: false, why: '', before: s.before, after: now };
	}
	return s;
})())`
}
