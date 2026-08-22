// Package react adds and removes reactions on messages.
//
// THE POSTCONDITION HAD TO BE DISCOVERED, not copied. Measured 2026-08-20 on
// the lab account: of 395 messages, ZERO carried a reaction and the
// ReactionsCollection was empty. There was no existing case to learn the shape
// from, so the first live run is what established which field moves.
//
// THE ARGUMENT IS THE MESSAGE MODEL, not its id — read from the app's own call,
// sendReactionToMsg(msg, emoji). Three capabilities in one day paid a
// correction each for assuming the other way (H40, H46, H49), and the pattern
// is now written down: when a page function dies reading a field off undefined,
// the argument is a model.
//
// REMOVING IS THE SAME CALL with an empty emoji, which is why Remove is a thin
// wrapper rather than a separate path — two paths would be two places for the
// same protocol detail to drift.
package react

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
	reactBudget = 30 * time.Second
	reactTick   = 500 * time.Millisecond
)

var (
	// ErrReact is the page refusing or throwing.
	ErrReact = fmt.Errorf("react: the page refused the reaction")
	// ErrNoMessage is an id that is not in the collection. It is distinct from
	// a refusal because the repairs differ: one is a wrong id, the other is a
	// page that would not act.
	ErrNoMessage = fmt.Errorf("react: no such message in the loaded collection")
	// ErrNotReactable is a message the page itself says cannot carry a
	// reaction. Asking anyway would be asking the app to break its own rule.
	ErrNotReactable = fmt.Errorf("react: this message cannot carry a reaction")
	// ErrUnreacted is the postcondition: the call returned and the message
	// shows no reaction.
	ErrUnreacted = fmt.Errorf("react: the page accepted the reaction and the message does not carry one")
)

// Result is what a reaction did.
type Result struct {
	// Had says whether the message already carried a reaction before the call.
	Had bool
	// Has says whether it carries one after.
	Has bool
	// Verified says whether the reported state was CHECKED against the page or
	// merely accepted from the call.
	//
	// It exists because the two halves of this capability differ and a caller
	// must not have to guess which they got: adding is verified, removing is
	// not (this build's hasReaction is sticky within the session that removed).
	// A Result that hid the difference would let "removed" read as strongly as
	// "added", which is not what the evidence supports.
	Verified bool
	// Waited is how long the page took to reflect it.
	Waited time.Duration
}

func (r Result) String() string {
	return fmt.Sprintf("react.Result(had=%t has=%t verified=%t waited=%s)",
		r.Had, r.Has, r.Verified, r.Waited.Round(time.Millisecond))
}

// Reactor reacts to messages on one session.
type Reactor struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Reactor.
func New(runner *engine.Runner, eval spa.Evaluator) *Reactor {
	return &Reactor{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessReact"

// Add puts an emoji on a message and returns only after the page shows it.
func (r *Reactor) Add(ctx context.Context, msgID, emoji, label string) (Result, error) {
	if strings.TrimSpace(emoji) == "" {
		return Result{}, fmt.Errorf("%w: adding a reaction needs an emoji; use Remove to clear one", ErrReact)
	}
	return r.set(ctx, msgID, emoji, true, label)
}

// Remove clears a reaction. It is the SAME page call with an empty emoji, not a
// second one — the protocol says so, and giving it its own path would be giving
// the same detail two places to drift.
//
// IT DOES NOT VERIFY, AND THAT IS MEASURED RATHER THAN CONCEDED. Removal works:
// three separate FRESH sessions read the account after a removal and found
// hasReaction false on every message and ZERO rows in the ReactionsCollection.
// What does not work is checking it from the session that performed it —
// hasReaction is STICKY there, staying true for at least 30 seconds of polling,
// and getReactionEmojisAndSum did not return an aggregate this code could read.
//
// So the honest contract is: Remove reports whether the PAGE ACCEPTED the call,
// and says here that the effect is confirmed only by re-reading in a new
// session. Polling a flag that never flips would have produced a capability
// that always fails at something that always works — worse than not checking.
func (r *Reactor) Remove(ctx context.Context, msgID, label string) (Result, error) {
	return r.set(ctx, msgID, "", false, label)
}

func (r *Reactor) set(ctx context.Context, msgID, emoji string, want bool, label string) (Result, error) {
	if strings.TrimSpace(msgID) == "" {
		return Result{}, ErrNoMessage
	}
	start := time.Now()

	var kicked string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return r.eval(ctx, reactScript(msgID, emoji), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrReact, err)
	}

	var out struct {
		Stage string `json:"stage"`
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		Had   bool   `json:"had"`
	}
	deadline := time.Now().Add(reactBudget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return r.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrReact, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("react: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrReact, reactBudget)
		}
		time.Sleep(reactTick)
	}
	switch {
	case out.Stage == "find" && !out.OK:
		if out.Why == "NOT_REACTABLE" {
			return Result{}, ErrNotReactable
		}
		return Result{}, fmt.Errorf("%w (%s)", ErrNoMessage, out.Why)
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrReact, out.Stage, out.Why)
	}

	// ADDING IS VERIFIED; REMOVING IS NOT, and the asymmetry is measured.
	//
	// Adding flips hasReaction false -> true within a second, so waiting for it
	// is a real postcondition. Removing does not flip it back in the same
	// session — see Remove's comment — so waiting would be waiting forever for
	// something that already happened.
	if !want {
		return Result{Had: out.Had, Has: false, Verified: false, Waited: time.Since(start)}, nil
	}
	has, err := r.waitFor(ctx, msgID, true, label)
	if err != nil {
		return Result{}, err
	}
	return Result{Had: out.Had, Has: has, Verified: true, Waited: time.Since(start)}, nil
}

// waitFor polls the message's reaction state until it matches want.
func (r *Reactor) waitFor(ctx context.Context, msgID string, want bool, label string) (bool, error) {
	deadline := time.Now().Add(reactBudget)
	var last bool
	var lastSticky bool
	var lastSum int
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/verify", func(ctx context.Context) error {
			return r.eval(ctx, readScript(msgID), &raw)
		}); err != nil {
			return false, fmt.Errorf("%w: %v", ErrReact, err)
		}
		var got struct {
			Found  bool `json:"found"`
			Has    bool `json:"has"`
			Sticky bool `json:"sticky"`
			Sum    int  `json:"sum"`
		}
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			return false, fmt.Errorf("react: unexpected verify answer: %w", err)
		}
		if !got.Found {
			return false, ErrNoMessage
		}
		lastSticky, lastSum = got.Sticky, got.Sum
		last = got.Has
		if got.Has == want {
			return got.Has, nil
		}
		if !time.Now().Before(deadline) {
			if want {
				return last, fmt.Errorf("%w (sticky=%t aggregate=%d)", ErrUnreacted, lastSticky, lastSum)
			}
			return last, fmt.Errorf("%w: the reaction is still there after being removed "+
				"(sticky=%t aggregate=%d)", ErrReact, lastSticky, lastSum)
		}
		time.Sleep(reactTick)
	}
}

// readScript reports only whether the message currently carries a reaction. It
// is deliberately separate from the acting script: the clock lives in Go, so
// the page is asked repeatedly rather than asked to wait.
func readScript(msgID string) string {
	return `JSON.stringify((() => {
		const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
		const want = ` + strconv.Quote(msgID) + `;
		for (const m of coll.getModelsArray()) {
			try {
				if (!m.id || m.id.id !== want) { continue; }
				// hasReaction is STICKY within a session: it stays true after a
				// reaction is removed, and a fresh session shows the message
				// clean. So it answers "has ever had one", which is the wrong
				// question for a removal — and it is, nevertheless, the only
				// signal this build offers.
				//
				// THERE WAS AN AGGREGATE BRANCH HERE AND IT NEVER RAN (H83).
				// It called getReactionEmojisAndSum(m) inside a try/catch, and
				// that function takes a LIST of records carrying .reactions —
				// not a message. Every call threw, the catch swallowed it, sum
				// stayed -1, and the fallback to the sticky flag decided every
				// answer.
				//
				// The comment above it claimed the aggregate was "the
				// display-level truth ... and it is what this reports". That
				// described code that never executed, which is worse than no
				// comment: it told the next reader the verification was stronger
				// than it was. The branch is gone and the honest signal is named.
				return { found: true, sticky: !!m.hasReaction, sum: -1,
					has: !!m.hasReaction };
			} catch (e) {}
		}
		return { found: false, has: false, sticky: false, sum: -1 };
	})())`
}

func reactScript(msgID, emoji string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			const want = ` + strconv.Quote(msgID) + `;
			let msg = null;
			for (const m of coll.getModelsArray()) {
				try { if (m.id && m.id.id === want) { msg = m; break; } } catch (e) {}
			}
			if (!msg) { park({ stage, ok: false, why: 'NOT_LOADED' }); return; }

			// The app's own rule about what can carry a reaction. Asking
			// otherwise would be asking it to break its own guard.
			try {
				const U = window.require('WAWebReactionsUtils');
				if (U && typeof U.canReactToMessage === 'function' && !U.canReactToMessage(msg)) {
					park({ stage, ok: false, why: 'NOT_REACTABLE' });
					return;
				}
			} catch (e) {}

			const had = !!msg.hasReaction;

			stage = 'react';
			const A = window.require('` + string(spa.ModuleSendReactionMsgAction) + `');
			// THE MODEL, not the id. An empty emoji removes.
			await A.sendReactionToMsg(msg, ` + strconv.Quote(emoji) + `);

			// NOT VERIFIED HERE. The page updates hasReaction ASYNCHRONOUSLY
			// after the call resolves, and reading it in the same breath is a
			// race: the first version did exactly that and reported "the
			// reaction is still there after being removed" for a removal that
			// had worked — a fresh session showed the message clean.
			//
			// Same lesson as the presence subscription (H50). The Go side owns
			// the clock (invariant 6), so it polls readScript instead.
			park({ stage: 'done', ok: true, why: '', had: had });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'react', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
