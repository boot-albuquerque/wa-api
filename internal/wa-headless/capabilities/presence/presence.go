// Package presence announces this account's state and observes other people's.
//
// TWO HALVES THAT FAIL DIFFERENTLY, which is why they are separate calls:
// announcing has NO local evidence — the page accepts it and only the other
// side can say whether anything arrived — while observing needs a subscription
// that must be asked for.
//
// Measured 2026-08-20 on the lab account: of 384 presence models, ZERO were
// subscribed. Reading the collection without subscribing would report everyone
// as permanently silent, and the code would look like it worked.
//
// THE LAYER CHOICE IS DELIBERATE. WAWebChatStateBridge takes a wid and sends
// the protocol directly; WAWebPresenceChatAction takes a CHAT and applies the
// app's own guards — getIsNewsletter, id.isBot(), getIsBroadcast — before doing
// the same thing, and maintains the resend timers a real client keeps. Using
// the bridge would mean deciding we know better than the app about where a
// typing indicator belongs. We do not.
package presence

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
	opBudget = 30 * time.Second
	opTick   = 500 * time.Millisecond
	// subscribeBudget bounds the wait for a subscription to be reflected.
	// Measured: subscribeUserPresence returns before isSubscribed flips, so a
	// read in the same breath is a race — it passed once and failed the next
	// run.
	subscribeBudget = 20 * time.Second
	subscribeTick   = time.Second
)

var (
	// ErrPresence is the page refusing or throwing.
	ErrPresence = fmt.Errorf("presence: the page refused")
	// ErrNoChat is a recipient that did not resolve.
	ErrNoChat = fmt.Errorf("presence: the recipient did not resolve to a chat")
	// ErrUnknownState is a state this package does not send. It is an error
	// rather than a silent default because "we sent nothing" and "we sent
	// paused" look identical from here.
	ErrUnknownState = fmt.Errorf("presence: unknown state")
	// ErrNotSubscribed is an observation of somebody whose presence was never
	// asked for. Returning "offline, not typing" there would be a fact-shaped
	// guess.
	ErrNotSubscribed = fmt.Errorf("presence: not subscribed to this presence")
)

// State is what this account announces it is doing in a chat.
type State string

const (
	// StateComposing is the typing indicator.
	StateComposing State = "composing"
	// StatePaused stops it. It is a distinct announcement, not the absence of
	// one: a client that only ever sent composing would leave the other side
	// showing a typing indicator forever.
	StatePaused State = "paused"
	// StateRecording is the voice-note indicator.
	StateRecording State = "recording"
)

// action maps a state to the page function that announces it.
//
// The mapping lives here rather than being built from the state string, so a
// state this package does not know cannot become a call to a function that does
// not exist.
var action = map[State]string{
	StateComposing: "markComposing",
	StatePaused:    "markPaused",
	StateRecording: "markRecording",
}

// Snapshot is what is known about one presence at one instant.
type Snapshot struct {
	// Subscribed says whether this account ever asked for this presence.
	// Everything below is meaningless when it is false, which is why it is
	// first.
	Subscribed bool
	Online     bool
	// ChatState is the page's own word: measured values include "composing",
	// "recording", "paused", "available" and "unavailable". It is carried
	// verbatim rather than mapped, because a value this package has not seen
	// must not be silently turned into one it has.
	ChatState string
}

func (s Snapshot) String() string {
	return fmt.Sprintf("presence.Snapshot(subscribed=%t online=%t chatstate=%q)",
		s.Subscribed, s.Online, s.ChatState)
}

// Typing reports whether the observed party is composing right now.
func (s Snapshot) Typing() bool { return s.ChatState == string(StateComposing) }

// Announcer announces and observes presence on one session.
type Announcer struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds an Announcer.
func New(runner *engine.Runner, eval spa.Evaluator) *Announcer {
	return &Announcer{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessPresence"

// Set announces a state in the chat with toJID.
//
// IT HAS NO LOCAL POSTCONDITION, and saying so is the honest part: the page
// accepts the call and only the OTHER side can report whether anything arrived.
// The closed-loop test between two accounts is what verifies this, and nothing
// here should be read as proof on its own.
func (a *Announcer) Set(ctx context.Context, toJID string, s State, label string) error {
	fn, ok := action[s]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownState, s)
	}
	if strings.TrimSpace(toJID) == "" {
		return ErrNoChat
	}
	out, err := a.run(ctx, setScript(toJID, fn), label)
	if err != nil {
		return err
	}
	if out.Stage == "resolve" && !out.OK {
		return fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
	}
	if !out.OK {
		return fmt.Errorf("%w at %s (%s)", ErrPresence, out.Stage, out.Why)
	}
	return nil
}

// SetOnline announces the ACCOUNT as available or not.
//
// It is separate from Set because it is not about a chat: the page's own calls
// take no argument at all, and the state is global to the session.
func (a *Announcer) SetOnline(ctx context.Context, available bool, label string) error {
	fn := "sendPresenceUnavailable"
	if available {
		fn = "sendPresenceAvailable"
	}
	out, err := a.run(ctx, onlineScript(fn), label)
	if err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("%w at %s (%s)", ErrPresence, out.Stage, out.Why)
	}
	return nil
}

// Observe subscribes to somebody's presence and reports what is known.
//
// The subscription is not optional and not cached here: measured, zero of 384
// presence models were subscribed, so a read without one reports silence that
// is indistinguishable from the real thing.
func (a *Announcer) Observe(ctx context.Context, ofJID, label string) (Snapshot, error) {
	if strings.TrimSpace(ofJID) == "" {
		return Snapshot{}, ErrNoChat
	}
	// THE SUBSCRIPTION DOES NOT LAND INSTANTLY, and the first version of this
	// method assumed it did: it subscribed and read in the same page call, then
	// reported ErrNotSubscribed for a presence that became subscribed a moment
	// later. It passed once by luck and failed the next run, which is the worst
	// kind of correct.
	//
	// So the read is retried until the page agrees it is subscribed. The clock
	// stays on THIS side (invariant 6): the page is asked repeatedly rather than
	// asked to wait.
	deadline := time.Now().Add(subscribeBudget)
	var out wireOut
	for {
		var err error
		out, err = a.run(ctx, observeScript(ofJID), label)
		if err != nil {
			return Snapshot{}, err
		}
		if out.Stage == "resolve" && !out.OK {
			return Snapshot{}, fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
		}
		if !out.OK {
			return Snapshot{}, fmt.Errorf("%w at %s (%s)", ErrPresence, out.Stage, out.Why)
		}
		if out.Subscribed {
			break
		}
		if !time.Now().Before(deadline) {
			return Snapshot{Subscribed: false}, ErrNotSubscribed
		}
		time.Sleep(subscribeTick)
	}
	return Snapshot{Subscribed: true, Online: out.Online, ChatState: out.ChatState}, nil
}

type wireOut struct {
	Stage      string `json:"stage"`
	OK         bool   `json:"ok"`
	Why        string `json:"why"`
	Subscribed bool   `json:"subscribed"`
	Online     bool   `json:"online"`
	ChatState  string `json:"chatstate"`
}

func (a *Announcer) run(ctx context.Context, script, label string) (wireOut, error) {
	var kicked string
	if err := a.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return a.eval(ctx, script, &kicked)
	}); err != nil {
		return wireOut{}, fmt.Errorf("%w: %v", ErrPresence, err)
	}
	deadline := time.Now().Add(opBudget)
	for {
		var raw string
		if err := a.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return a.eval(ctx, resultScript, &raw)
		}); err != nil {
			return wireOut{}, fmt.Errorf("%w: %v", ErrPresence, err)
		}
		var out wireOut
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return wireOut{}, fmt.Errorf("presence: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			return out, nil
		}
		if !time.Now().Before(deadline) {
			return wireOut{}, fmt.Errorf("%w: the page never settled within %s", ErrPresence, opBudget)
		}
		time.Sleep(opTick)
	}
}

// chatLookupExpr finds the chat for a jid WITHOUT creating one.
//
// Announcing presence must never bring a conversation into existence: telling
// somebody you are typing is not the same as opening a chat with them, and a
// capability that did both would create chats as a side effect of a status
// update.
// It RESOLVES the identity first, because this build files chats under the
// identity the server assigns and not under the number a caller types. The
// first version looked the chat up by the raw jid and the page answered
// NO_CHAT for two accounts that talk to each other every day (H34, H39).
const chatLookupExpr = `(async function (jidString) {
	const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
	const resolveIdentity = ` + spa.ResolveIdentityExpr + `;
	const r = await resolveIdentity(jidString);
	if (!r.ok) { return r; }
	const chat = Chats.get(r.wid);
	if (!chat) { return { ok: false, why: 'NO_CHAT' }; }
	return { ok: true, wid: r.wid, jid: r.jid, chat: chat };
})`

func setScript(toJID, fn string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		const lookup = ` + chatLookupExpr + `;
		(async () => {
		let stage = 'resolve';
		try {
			const r = await lookup(` + strconv.Quote(toJID) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }
			stage = 'announce';
			// The CHAT action, not the raw bridge: it applies the app's guards
			// and keeps the resend timers a real client keeps.
			const A = window.require('` + string(spa.ModulePresenceChatAction) + `');
			await A.` + fn + `(r.chat);
			park({ stage: 'done', ok: true, why: '' });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

func onlineScript(fn string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
		let stage = 'announce';
		try {
			const A = window.require('` + string(spa.ModulePresenceChatAction) + `');
			await A.` + fn + `();
			park({ stage: 'done', ok: true, why: '' });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

func observeScript(ofJID string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		const lookup = ` + chatLookupExpr + `;
		(async () => {
		let stage = 'resolve';
		try {
			const r = await lookup(` + strconv.Quote(ofJID) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }
			stage = 'subscribe';
			const B = window.require('` + string(spa.ModuleContactPresenceBridge) + `');
			await B.subscribeUserPresence(r.wid);
			stage = 'read';
			const coll = window.require('` + string(spa.ModulePresenceCollection) + `').PresenceCollection;
			const p = coll.get(r.wid);
			if (!p) { park({ stage: 'done', ok: true, why: '', subscribed: false }); return; }
			let cs = '';
			try { if (p.chatstate && p.chatstate.type != null) { cs = String(p.chatstate.type); } } catch (e) {}
			park({
				stage: 'done', ok: true, why: '',
				subscribed: !!p.isSubscribed, online: !!p.isOnline, chatstate: cs
			});
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'announce', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
