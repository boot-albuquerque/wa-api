// Package pin pins and unpins a MESSAGE inside a conversation.
//
// NOT PROVEN AGAINST THE LIVE BUILD — see H81. The call is accepted and nothing
// is pinned: a fresh session reads PinInChatCollection as empty and no message
// carries a pin. Nothing was left pinned by the attempt, which was checked.
//
// IT IS NOT PINNING A CONVERSATION. Those are different acts with different
// primitives — a pinned chat sits at the top of the list, a pinned message sits
// at the top of one conversation — and the chat one lives in capabilities/chats.
// The upstream names them Message.pin and Chat.pin, one letter apart in a
// sentence and nothing alike underneath.
//
// THE SHAPE AND THE VOCABULARY WERE BOTH MEASURED:
//
//	sendPinInChatMsg(msg, state, seconds)
//	PIN_STATE                        = {INVALID: 0, PIN: 1, UNPIN: 2}   // UI
//	Message$PinInChatMessage$Type    = {UNKNOWN_TYPE: 0, PIN_FOR_ALL: 1,
//	                                    UNPIN_FOR_ALL: 2}               // wire
//	DEFAULT_PIN_EXPIRY_DURATION_OPTION = "SevenDays" -> 604800 seconds
//
// THE APP'S OWN CALLER USES THE WIRE ENUM, from WAWebProtobufsE2E.pb, and passes
// only two arguments when unpinning. Finding that felt like the answer — and the
// two enums carry the SAME NUMBERS, so it changed nothing. Worth writing down
// precisely because it looked decisive and was not.
//
// The state numbers are READ FROM THE PAGE rather than written here, for the
// reason the ack capability gives: a constant nobody can check is a constant
// that goes wrong quietly.
//
// A PIN EXPIRES. That is unusual enough to be worth saying in the type: Pinned
// carries the duration, because "pinned" without "until when" is a fact with
// half its meaning missing.
package pin

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/spa"
)

// Bounds. Var, not const, so tests can compress the clock.
var (
	pinBudget = 30 * time.Second
	pinTick   = 500 * time.Millisecond
)

var (
	// ErrPin is the page refusing or throwing.
	ErrPin = fmt.Errorf("pin: the page refused")
	// ErrNoMessage is an id that is not in the loaded collection.
	ErrNoMessage = fmt.Errorf("pin: no such message in the loaded collection")
	// ErrNoChat is a jid with no loaded chat.
	ErrNoChat = fmt.Errorf("pin: no such chat is loaded")
	// ErrNoVocabulary is the page not exposing PIN_STATE. It is its own error
	// because inventing 1 and 2 would work until the day it did not.
	ErrNoVocabulary = fmt.Errorf("pin: this build does not expose the pin-state vocabulary")
)

// Pinned is what pinning or unpinning did.
type Pinned struct {
	// MessageID is what was acted on.
	MessageID string
	// Wanted says which direction was asked for.
	Wanted bool
	// Seconds is how long the pin lasts. Zero when unpinning.
	Seconds int
	// StateSource says where the state numbers came from, or "absent".
	StateSource string
	// Verified is false for a real change: this build does not show the session
	// that pinned a message that it did. PinnedIn read from a LATER session is
	// the honest proof — the same shape as group membership (H58).
	Verified bool
	Waited   time.Duration
}

func (p Pinned) String() string {
	return fmt.Sprintf("pin.Pinned(id=%s wanted=%t seconds=%d states=%s verified=%t waited=%s)",
		p.MessageID, p.Wanted, p.Seconds, p.StateSource, p.Verified, p.Waited.Round(time.Millisecond))
}

// Pinner pins messages on one session.
type Pinner struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Pinner.
func New(runner *engine.Runner, eval spa.Evaluator) *Pinner {
	return &Pinner{runner: runner, eval: eval}
}

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other, and each polled it
// until it stopped saying "pending" — so one could take the other's answer. The
// nonce comes from Go: a page-side Math.random or Date.now would put a decision
// and a clock where invariant 6 forbids them.
const stateKeyPrefix = "__headlessPinMsg"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// Message pins a message for the build's default duration.
func (p *Pinner) Message(ctx context.Context, msgID, label string) (Pinned, error) {
	return p.set(ctx, msgID, true, label)
}

// Unpin takes it down.
func (p *Pinner) Unpin(ctx context.Context, msgID, label string) (Pinned, error) {
	return p.set(ctx, msgID, false, label)
}

func (p *Pinner) set(ctx context.Context, msgID string, on bool, label string) (Pinned, error) {
	if strings.TrimSpace(msgID) == "" {
		return Pinned{}, ErrNoMessage
	}
	start := time.Now()

	key := nextStateKey()

	var kicked string
	if err := p.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return p.eval(ctx, pinScript(msgID, on, key), &kicked)
	}); err != nil {
		return Pinned{}, fmt.Errorf("%w: %v", ErrPin, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Seconds int    `json:"seconds"`
		Source  string `json:"source"`
	}
	deadline := time.Now().Add(pinBudget)
	for {
		var raw string
		if err := p.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return p.eval(ctx, resultScript(key), &raw)
		}); err != nil {
			return Pinned{}, fmt.Errorf("%w: %v", ErrPin, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Pinned{}, fmt.Errorf("pin: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			// A CHAVE E' LIBERADA ao sair do laco (H177): sem isso a correcao
			// troca uma resposta cruzada por um global de pagina POR CHAMADA.
			var ignored string
			_ = p.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return p.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			break
		}
		if !time.Now().Before(deadline) {
			return Pinned{}, fmt.Errorf("%w: the page never settled within %s", ErrPin, pinBudget)
		}
		time.Sleep(pinTick)
	}

	switch {
	case out.Why == "NOT_LOADED":
		return Pinned{}, ErrNoMessage
	case out.Why == "NO_VOCABULARY":
		return Pinned{}, ErrNoVocabulary
	case !out.OK:
		return Pinned{}, fmt.Errorf("%w at %s (%s)", ErrPin, out.Stage, out.Why)
	}
	return Pinned{MessageID: msgID, Wanted: on, Seconds: out.Seconds,
		StateSource: out.Source, Verified: false, Waited: time.Since(start)}, nil
}

// PinnedIn lists the message ids pinned in a conversation.
//
// WHETHER IT GOES STALE AFTER A PIN IS UNKNOWN, AND SAYING SO IS THE POINT.
//
// This doc used to state that it is "correct at session start and stale after a
// change this session made", which is a claim about what happens after a
// successful pin — and no successful pin has ever been observed here. Pinning
// is accepted by the page and nothing is pinned (H81), so the sentence
// described the aftermath of an event that never occurred. It was inherited
// from H58's participant measurement, the same over-broad generalisation that
// had to be corrected in group.PolicyOf (H90).
//
// What IS known: the reader works, and the account has nothing pinned. When a
// pin succeeds, the write class can be measured with spa.ClassifyWriteExpr like
// any other, and this comment can then say something it has evidence for.
func (p *Pinner) PinnedIn(ctx context.Context, chatJID, label string) ([]string, error) {
	if strings.TrimSpace(chatJID) == "" {
		return nil, ErrNoChat
	}
	var raw string
	if err := p.runner.Do(ctx, engine.OpStateProbe, label+"/pinned", func(ctx context.Context) error {
		return p.eval(ctx, pinnedInScript(chatJID), &raw)
	}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPin, err)
	}
	var out struct {
		OK  bool     `json:"ok"`
		Why string   `json:"why"`
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("pin: unexpected pinned-list answer: %w", err)
	}
	if !out.OK {
		if out.Why == "NO_CHAT" {
			return nil, ErrNoChat
		}
		return nil, fmt.Errorf("%w (%s)", ErrPin, out.Why)
	}
	// A CONVERSATION WITH NOTHING PINNED RETURNS AN EMPTY SLICE, never nil:
	// "nothing is pinned here" is an answer.
	if out.IDs == nil {
		return []string{}, nil
	}
	return out.IDs, nil
}

func pinScript(msgID string, on bool, key string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(key) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(key) + `] = v; };
		(async () => {
		let stage = 'find';
		try {
			const MC = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			let msg = null;
			for (const m of MC.getModelsArray()) {
				try { if (m.id && m.id.id === ` + strconv.Quote(msgID) + `) { msg = m; break; } } catch (e) {}
			}
			if (!msg) { park({ stage, ok: false, why: 'NOT_LOADED' }); return; }

			stage = 'vocabulary';
			// THE NUMBERS COME FROM THE PAGE. Writing 1 and 2 here would work
			// until a build renumbered them, and then it would pin when asked to
			// unpin — a failure nobody would read as a renumbered enum.
			const K = window.require('` + string(spa.ModulePinMsgConstants) + `');
			const states = K.PIN_STATE;
			if (!states || typeof states.PIN !== 'number' || typeof states.UNPIN !== 'number') {
				park({ stage, ok: false, why: 'NO_VOCABULARY' }); return;
			}
			const on = ` + strconv.FormatBool(on) + `;
			const state = on ? states.PIN : states.UNPIN;

			// THE DURATION comes from the page's own default option, converted
			// by the page's own function. A pin EXPIRES, and choosing the number
			// here would be choosing how long other people see it.
			let seconds = 0;
			if (on) {
				try {
					const opt = K.DEFAULT_PIN_EXPIRY_DURATION_OPTION;
					seconds = Number(K.getPinExpiryDuration(opt)) || 0;
				} catch (e) { seconds = 0; }
			}

			stage = 'apply';
			const A = window.require('` + string(spa.ModuleSendPinMessageAction) + `');
			// THREE POSITIONAL, and the first is the message MODEL — measured:
			// it reads id, to, from, revisionNumber and id.remote._serialized.
			await A.sendPinInChatMsg(msg, state, seconds);

			// NO POSTCONDITION HERE. The pin collection does not reflect a
			// change this session made, the same wall H58 measured for group
			// metadata; PinnedIn from a later session is the honest proof.
			park({ stage: 'done', ok: true, why: '', seconds: seconds, source: 'WAWebPinMsgConstants.PIN_STATE' });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

func pinnedInScript(chatJID string) string {
	return `JSON.stringify((() => {
		try {
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const chat = CC.get(` + strconv.Quote(chatJID) + `);
			if (!chat) { return { ok: false, why: 'NO_CHAT', ids: [] }; }
			const key = chat.id.toString();
			const P = window.require('` + string(spa.ModulePinInChatCollection) + `');
			const coll = P.PinInChatCollection;
			const arr = (coll && coll.getModelsArray) ? coll.getModelsArray() : [];
			const ids = [];
			for (const m of arr) {
				try {
					// WHICH FIELD CARRIES THE PARENT was never established, so
					// the plausible ones are tried and the first that matches
					// this chat wins. Picking one would work until it did not.
					const owner = (m.parentMsgKey && m.parentMsgKey.remote) ||
						(m.chatId) || (m.id && m.id.remote);
					if (!owner || String(owner) !== key) { continue; }
					const id = (m.parentMsgKey && m.parentMsgKey.id) ||
						(m.parentMsgId) || (m.id && m.id.id);
					if (id) { ids.push(String(id)); }
				} catch (e) {}
			}
			return { ok: true, why: '', ids: ids };
		} catch (e) {
			return { ok: false, why: String((e && e.message) || e).slice(0, 160), ids: [] };
		}
	})())`
}

func resultScript(key string) string {
	return `JSON.stringify((() => {
	const s = window[` + strconv.Quote(key) + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
}
