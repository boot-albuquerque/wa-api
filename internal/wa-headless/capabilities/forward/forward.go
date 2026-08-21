// Package forward re-sends an existing message to another chat.
//
// THE SHAPE CAME FROM A CALL SITE, not from the function. Both exported
// functions are async wrappers taking a single opaque `e`, and unlike starring
// and muting there was no model method to fall back on — the chat model has no
// forward method. What the app's own forward flow builds is:
//
//	forwardMessagesToChats({msgs, chats, includeCaption, appendedText})
//
// with `chats` an array of CHAT MODELS and `msgs` an array of MESSAGE MODELS.
//
// THE POSTCONDITION IS A NEW MESSAGE, AND IT IS DELIBERATELY STRICT. An earlier
// closed-loop check in this module accepted a stranger's message that arrived
// 23 seconds BEFORE the send, which is why "a message appeared" is not the test.
// The forwarded copy has to be one this account sent, in the target chat, whose
// id was NOT present before the call. All three, or it does not count.
package forward

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
	forwardBudget = 30 * time.Second
	forwardTick   = 500 * time.Millisecond
)

var (
	// ErrForward is the page refusing or throwing.
	ErrForward = fmt.Errorf("forward: the page refused")
	// ErrNoMessage is a source id that is not in the loaded collection.
	ErrNoMessage = fmt.Errorf("forward: no such message in the loaded collection")
	// ErrNoChat is a target jid with no loaded chat. Forwarding does NOT create
	// one: opening a conversation with somebody in order to re-send them
	// somebody else's message is a bigger act than the caller asked for.
	ErrNoChat = fmt.Errorf("forward: no such chat is loaded; this capability does not create one")
	// ErrNotForwardable is the page saying this message cannot be forwarded.
	ErrNotForwardable = fmt.Errorf("forward: this build says this message cannot be forwarded")
	// ErrNotDelivered is the postcondition: the call returned and no new
	// message of ours is in the target chat.
	ErrNotDelivered = fmt.Errorf("forward: the page accepted the forward and no new message of ours appeared in the target chat")
)

// Result is a forwarded copy.
type Result struct {
	// NewID is the id of the copy in the target chat. It is an id, not content.
	NewID string
	// BodyLen is how long the forwarded body is, reported instead of the body.
	BodyLen int
	// Waited is how long the copy took to appear.
	Waited time.Duration
}

func (r Result) String() string {
	return fmt.Sprintf("forward.Result(newID=%s bodyLen=%d waited=%s)",
		r.NewID, r.BodyLen, r.Waited.Round(time.Millisecond))
}

// Forwarder forwards messages on one session.
type Forwarder struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Forwarder.
func New(runner *engine.Runner, eval spa.Evaluator) *Forwarder {
	return &Forwarder{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessForward"

// To forwards a message to a chat that is already loaded.
//
// includeCaption is a parameter rather than a constant because forwarding media
// with and without its caption are different acts, and choosing one on the
// caller's behalf would be choosing what other people read.
func (f *Forwarder) To(ctx context.Context, msgID, toChatJID string, includeCaption bool, label string) (Result, error) {
	if strings.TrimSpace(msgID) == "" {
		return Result{}, ErrNoMessage
	}
	if strings.TrimSpace(toChatJID) == "" {
		return Result{}, ErrNoChat
	}
	start := time.Now()

	var kicked string
	if err := f.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return f.eval(ctx, forwardScript(msgID, toChatJID, includeCaption), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrForward, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		NewID   string `json:"newID"`
		BodyLen int    `json:"bodyLen"`
	}
	deadline := time.Now().Add(forwardBudget)
	for {
		var raw string
		if err := f.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return f.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrForward, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("forward: unexpected answer: %w", err)
		}
		if out.Stage != "pending" && out.Stage != "settling" {
			break
		}
		if !time.Now().Before(deadline) {
			if out.Stage == "settling" {
				return Result{}, fmt.Errorf("%w within %s", ErrNotDelivered, forwardBudget)
			}
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrForward, forwardBudget)
		}
		time.Sleep(forwardTick)
	}

	switch {
	case out.Why == "NOT_LOADED":
		return Result{}, ErrNoMessage
	case out.Why == "NO_CHAT":
		return Result{}, ErrNoChat
	case out.Why == "NOT_FORWARDABLE":
		return Result{}, ErrNotForwardable
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrForward, out.Stage, out.Why)
	}
	if out.NewID == "" {
		return Result{}, ErrNotDelivered
	}
	return Result{NewID: out.NewID, BodyLen: out.BodyLen, Waited: time.Since(start)}, nil
}

func forwardScript(msgID, toChatJID string, includeCaption bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			let msg = null;
			for (const m of MC.getModelsArray()) {
				try { if (m.id && m.id.id === ` + strconv.Quote(msgID) + `) { msg = m; break; } } catch (e) {}
			}
			if (!msg) { park({ stage, ok: false, why: 'NOT_LOADED' }); return; }

			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(toChatJID) + `);
			// NO findOrCreateLatestChat here, on purpose: opening a conversation
			// with somebody in order to re-send them a message is a bigger act
			// than forwarding, and the caller did not ask for it.
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			if (msg.canForward === false) {
				park({ stage, ok: false, why: 'NOT_FORWARDABLE' }); return;
			}

			// EVERY ID ALREADY IN THE TARGET CHAT. The postcondition compares
			// against this set rather than against a timestamp, because a
			// timestamp comparison in this module once accepted a stranger's
			// message that arrived 23 seconds BEFORE the send.
			const chatKey = chat.id.toString();
			const seen = new Set();
			for (const m of MC.getModelsArray()) {
				try {
					if (m.id && m.id.remote && m.id.remote.toString() === chatKey) {
						seen.add(m.id.id);
					}
				} catch (e) {}
			}

			stage = 'apply';
			const F = window.require('` + string(spa.ModuleForwardMessagesToChat) + `');
			// CHAT MODELS and MESSAGE MODELS, both in arrays — read from the
			// app's own call site, because neither function's toString() shows
			// anything but an opaque single argument.
			await F.forwardMessagesToChats({
				msgs: [msg], chats: [chat],
				includeCaption: ` + strconv.FormatBool(includeCaption) + `
			});

			stage = 'verify';
			park({ stage: 'settling', ok: true, why: '',
				chatKey: chatKey, seen: seen });
		} catch (e) {
			// The app's forward errors carry a reasons field; flattening it to
			// "failed" would throw away the only part a caller can act on.
			// (No backticks in this comment: it lives inside a Go raw string,
			// and one would end the literal.)
			let why = String((e && e.message) || e);
			try {
				if (e && e.reasons) { why += ' reasons=' + JSON.stringify(e.reasons); }
			} catch (e2) {}
			park({ stage, ok: false, why: why.slice(0, 200) });
		}
		})();
		return { started: true };
	})())`
}

// resultScript does the comparison in the page because the id set lives there,
// and returns only an id and a length.
const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	if (s.stage === 'settling') {
		const MC = window.require('WAWebMsgCollection').MsgCollection;
		for (const m of MC.getModelsArray()) {
			try {
				if (!m.id || !m.id.fromMe) { continue; }
				if (!m.id.remote || m.id.remote.toString() !== s.chatKey) { continue; }
				if (s.seen.has(m.id.id)) { continue; }
				return { stage: 'done', ok: true, why: '',
					newID: m.id.id, bodyLen: (m.body || '').length };
			} catch (e) {}
		}
		return { stage: 'settling', ok: false, why: '' };
	}
	return s;
})())`
