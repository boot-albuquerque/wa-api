// Package star marks and unmarks a message as starred.
//
// THE CALL SHAPE WAS SETTLED BY EXPERIMENT, not by reading, and the distinction
// is recorded because it cost a probe: sendStarMsgs is an async wrapper whose
// toString() shows only `function u(e,t,n){return d(t,n)}` — enough to know the
// first argument is discarded, not enough to know the rest. Three candidate
// shapes were tried one at a time against a real message. The first won:
//
//	Cmd.sendStarMsgs(chat, [msg], true)
//	Cmd.sendUnstarMsgs(chat, [msg], true)
//
// THE POSTCONDITION IS msg.star, AND THAT IS A CORRECTION. The parity table had
// listed WAWebStarredMsgCollection, on the strength of its name. The experiment
// measured it THROWING — its count came back -1 before and after a star that
// demonstrably worked. A postcondition built on it would have approved
// everything, including failure.
//
// STARRING IS LOCAL. The peer never sees it, which is what makes this the
// safest outward effect in this whole surface and why it could be settled by
// experiment at all.
package star

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
	starBudget = 30 * time.Second
	starTick   = 500 * time.Millisecond
)

var (
	// ErrStar is the page refusing or throwing.
	ErrStar = fmt.Errorf("star: the page refused")
	// ErrNoMessage is an id that is not in the loaded collection.
	ErrNoMessage = fmt.Errorf("star: no such message in the loaded collection")
	// ErrNoChat is a message whose chat is not loaded. The call takes the chat
	// model, so this is a real precondition and not an internal detail.
	ErrNoChat = fmt.Errorf("star: the message's chat is not loaded")
	// ErrFlagUnchanged is the postcondition: the call returned and the flag on
	// the model did not move.
	ErrFlagUnchanged = fmt.Errorf("star: the page accepted the change and the flag did not move")
)

// Result is what a star or unstar did.
type Result struct {
	// Before and After are the flag on the message model.
	Before, After bool
	// AlreadyInState is true when there was nothing to do. A success, for the
	// same reason an already-archived chat is (H55).
	AlreadyInState bool
	// Waited is how long the page took to reflect it.
	Waited time.Duration
}

// Changed reports whether the flag actually moved.
func (r Result) Changed() bool { return r.Before != r.After }

func (r Result) String() string {
	return fmt.Sprintf("star.Result(before=%t after=%t already=%t waited=%s)",
		r.Before, r.After, r.AlreadyInState, r.Waited.Round(time.Millisecond))
}

// Starrer stars messages on one session.
type Starrer struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Starrer.
func New(runner *engine.Runner, eval spa.Evaluator) *Starrer {
	return &Starrer{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessStar"

// Star marks a message.
func (s *Starrer) Star(ctx context.Context, msgID, label string) (Result, error) {
	return s.set(ctx, msgID, true, label)
}

// Unstar unmarks it.
func (s *Starrer) Unstar(ctx context.Context, msgID, label string) (Result, error) {
	return s.set(ctx, msgID, false, label)
}

func (s *Starrer) set(ctx context.Context, msgID string, want bool, label string) (Result, error) {
	if strings.TrimSpace(msgID) == "" {
		return Result{}, ErrNoMessage
	}
	start := time.Now()

	var kicked string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return s.eval(ctx, starScript(msgID, want), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrStar, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Before  bool   `json:"before"`
		After   bool   `json:"after"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(starBudget)
	for {
		var raw string
		if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return s.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrStar, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("star: unexpected answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			// A settling stage that ran out of budget is not a page that hung —
			// it is a flag that never moved, and saying so names the defect
			// instead of blaming the clock.
			if out.Stage == "settling" {
				return Result{}, fmt.Errorf("%w within %s (before=%t after=%t)",
					ErrFlagUnchanged, starBudget, out.Before, out.After)
			}
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrStar, starBudget)
		}
		time.Sleep(starTick)
	}

	switch {
	case out.Why == "NOT_LOADED":
		return Result{}, ErrNoMessage
	case out.Why == "NO_CHAT":
		return Result{}, ErrNoChat
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrStar, out.Stage, out.Why)
	}

	res := Result{Before: out.Before, After: out.After,
		AlreadyInState: out.Already, Waited: time.Since(start)}
	if !out.Already && !res.Changed() {
		return Result{}, fmt.Errorf("%w (before=%t after=%t)", ErrFlagUnchanged, out.Before, out.After)
	}
	return res, nil
}

func starScript(msgID string, want bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			const want = ` + strconv.FormatBool(want) + `;
			let msg = null;
			for (const m of MC.getModelsArray()) {
				try { if (m.id && m.id.id === ` + strconv.Quote(msgID) + `) { msg = m; break; } } catch (e) {}
			}
			if (!msg) { park({ stage, ok: false, why: 'NOT_LOADED' }); return; }

			// THE CHAT MODEL, because the measured call takes it. Looking it up
			// by msg.id.remote is how the app itself reaches it.
			const chat = window.require('` + string(spa.ModuleChatCollection) + `')
				.ChatCollection.get(msg.id.remote);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			const before = !!msg.star;
			if (before === want) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true,
					before: before, after: before });
				return;
			}

			stage = 'apply';
			const Cmd = window.require('` + string(spa.ModuleCmdForStar) + `').Cmd;
			// CHAT MODEL and an ARRAY OF MESSAGE MODELS — measured by trying
			// three shapes one at a time, not inferred from the bridge's name.
			if (want) { await Cmd.sendStarMsgs(chat, [msg], true); }
			else { await Cmd.sendUnstarMsgs(chat, [msg], true); }

			stage = 'verify';
			// THE AWAIT IS NOT THE COMPLETION. Measured: sendStarMsgs resolves
			// before the model flips, and reading msg.star here returns the OLD
			// value — the live proof failed on exactly that, with
			// before=false after=false. The probe had only passed because it
			// slept 800ms.
			//
			// The message MODEL is parked instead, and Go polls it. That keeps
			// the clock out of the page (invariant 6): a setTimeout here would
			// work and would make the page's answer depend on a duration
			// nothing in Go could see or compress.
			park({ stage: 'settling', ok: true, why: '', already: false,
				before: before, want: want, msg: msg });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

// resultScript never serialises the parked message model — it reads one boolean
// off it and builds the answer itself. Handing a page model to JSON.stringify is
// how a probe finds out what a cyclic object graph costs.
const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		const now = !!(s.msg && s.msg.star);
		if (now === s.want) {
			return { stage: 'done', ok: true, why: '', already: false,
				before: s.before, after: now };
		}
		return { stage: 'settling', ok: false, why: '', before: s.before, after: now };
	}
	return s;
})())`
