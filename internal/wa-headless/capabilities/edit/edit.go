// Package edit changes the text of a message already sent.
//
// THE SIGNATURE IS READ, NOT GUESSED (probe_edit_test.go):
//
//	sendMessageEdit(msg, text, options)
//
// It is synchronous, so its own body is legible, and the body opens by
// rejecting unless canEditText or canEditCaption allows it. This package asks
// the same predicate first, because a rejected promise carrying "Cannot edit
// message" tells a caller nothing about WHY.
//
// THE WINDOW IS 1200 SECONDS on this build, measured rather than assumed, and
// it is the dominant reason an edit fails. Every message in the loaded
// collection at probe time was outside it. ErrNotEditable says the number.
//
// THE POSTCONDITION TOOK MEASURING TOO. latestEditMsgKey is DEFINED on messages
// that were never edited, so its presence proves nothing; only its value moving
// from null does. The body changing is the second half, and this package waits
// for both — one of them alone would accept a page that recorded the edit
// without applying it, or applied it without recording it.
package edit

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
	editBudget = 30 * time.Second
	editTick   = 500 * time.Millisecond
)

// MaxTextBytes is this package's own ceiling, not the app's. An edit that is
// really a rewrite is a different act, and a caller who sends a novel by
// accident should hear about it here rather than from the wire.
const MaxTextBytes = 64 << 10

var (
	// ErrEdit is the page refusing or throwing.
	ErrEdit = fmt.Errorf("edit: the page refused")
	// ErrNoMessage is an id that is not in the loaded collection.
	ErrNoMessage = fmt.Errorf("edit: no such message in the loaded collection")
	// ErrNotOurs is somebody else's message. Editing is only ever possible on
	// what this account sent, and saying so is more useful than the app's
	// generic refusal.
	ErrNotOurs = fmt.Errorf("edit: this account did not send that message")
	// ErrNotEditable is the page saying no — usually the window has passed.
	ErrNotEditable = fmt.Errorf("edit: this message can no longer be edited (this build allows 1200s after sending)")
	// ErrEmptyText is a caller trying to empty a message. Deleting is a
	// different act with a different capability, and doing it by accident
	// through this one is worth refusing.
	ErrEmptyText = fmt.Errorf("edit: empty replacement text; deleting is revoke, not edit")
	// ErrTooLong is the ceiling above.
	ErrTooLong = fmt.Errorf("edit: replacement text is larger than this package will send")
	// ErrUnchanged is the postcondition: the call returned and the message
	// still reads the way it did.
	ErrUnchanged = fmt.Errorf("edit: the page accepted the edit and the message did not change")
)

// Result is what an edit did. It carries no text — a message body is content,
// and this module reports shapes.
type Result struct {
	// FromLen and ToLen are body LENGTHS before and after.
	FromLen, ToLen int
	// Recorded is whether the page also stamped the edit on the model, which is
	// what a reader's client uses to show "edited". Applied-but-not-recorded is
	// a real state and worth distinguishing from success.
	Recorded bool
	// Waited is how long the page took to reflect it.
	Waited time.Duration
}

func (r Result) String() string {
	return fmt.Sprintf("edit.Result(fromLen=%d toLen=%d recorded=%t waited=%s)",
		r.FromLen, r.ToLen, r.Recorded, r.Waited.Round(time.Millisecond))
}

// Editor edits messages on one session.
type Editor struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds an Editor.
func New(runner *engine.Runner, eval spa.Evaluator) *Editor {
	return &Editor{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessEdit"

// Text replaces the text of a message this account sent.
func (e *Editor) Text(ctx context.Context, msgID, newText, label string) (Result, error) {
	if strings.TrimSpace(msgID) == "" {
		return Result{}, ErrNoMessage
	}
	if newText == "" {
		return Result{}, ErrEmptyText
	}
	if len(newText) > MaxTextBytes {
		return Result{}, fmt.Errorf("%w (%d bytes, ceiling %d)", ErrTooLong, len(newText), MaxTextBytes)
	}
	start := time.Now()

	var kicked string
	if err := e.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return e.eval(ctx, editScript(msgID, newText), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrEdit, err)
	}

	var out struct {
		Stage    string `json:"stage"`
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		FromLen  int    `json:"fromLen"`
		ToLen    int    `json:"toLen"`
		Applied  bool   `json:"applied"`
		Recorded bool   `json:"recorded"`
	}
	deadline := time.Now().Add(editBudget)
	for {
		var raw string
		if err := e.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return e.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrEdit, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("edit: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrEdit, editBudget)
		}
		time.Sleep(editTick)
	}

	switch {
	case out.Why == "NOT_LOADED":
		return Result{}, ErrNoMessage
	case out.Why == "NOT_OURS":
		return Result{}, ErrNotOurs
	case out.Why == "NOT_EDITABLE":
		return Result{}, ErrNotEditable
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrEdit, out.Stage, out.Why)
	}
	// THE POSTCONDITION, and it is the half the probe had to answer: the body
	// has to read the new way. Reporting an edit that did not land would leave
	// a caller believing readers see text they do not.
	if !out.Applied {
		return Result{}, fmt.Errorf("%w (fromLen=%d toLen=%d)", ErrUnchanged, out.FromLen, out.ToLen)
	}
	return Result{FromLen: out.FromLen, ToLen: out.ToLen,
		Recorded: out.Recorded, Waited: time.Since(start)}, nil
}

func editScript(msgID, newText string) string {
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
			if (!msg.id.fromMe) { park({ stage, ok: false, why: 'NOT_OURS' }); return; }

			stage = 'allowed';
			// THE APP'S OWN GATE, asked before the call rather than after.
			// sendMessageEdit opens by rejecting on exactly this, and its
			// rejection says only "Cannot edit message".
			const Cap = window.require('` + string(spa.ModuleMsgActionCapability) + `');
			const allowed = !!(Cap.canEditText && Cap.canEditText(msg)) ||
				!!(Cap.canEditCaption && Cap.canEditCaption(msg));
			if (!allowed) { park({ stage, ok: false, why: 'NOT_EDITABLE' }); return; }

			const fromLen = (msg.body || '').length;
			const text = ` + strconv.Quote(newText) + `;

			stage = 'apply';
			const A = window.require('` + string(spa.ModuleSendMessageEditAction) + `');
			// THREE POSITIONAL — measured. options is an empty object rather
			// than omitted: the app spreads it into the edit payload, and a
			// missing third argument is a different call than an empty one.
			await A.sendMessageEdit(msg, text, {});

			stage = 'verify';
			// BOTH halves. latestEditMsgKey is DEFINED on never-edited
			// messages, so only a non-null value means anything; and a
			// recorded edit whose body did not move is not an applied edit.
			park({ stage: 'done', ok: true, why: '',
				fromLen: fromLen,
				toLen: (msg.body || '').length,
				applied: (msg.body || '') === text,
				recorded: msg.latestEditMsgKey != null });
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
	return s;
})())`
