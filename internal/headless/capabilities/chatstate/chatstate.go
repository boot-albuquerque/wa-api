// Package chatstate archives and pins conversations.
//
// BOTH ARE REVERSIBLE AND BOTH ARE VERIFIED, which makes them the easiest
// operations in this module to be honest about: the flag they set is readable
// on this side, so "the page accepted it" never has to stand in for "it
// happened".
//
// THE FLAG IS POLLED, NOT READ ONCE. Reacting taught this the hard way (H53):
// page state updates asynchronously after an awaited call, and reading in the
// same breath is a race that passes once and fails next time.
//
// PINNING HAS A LIMIT. getPinLimit and getNumConversationsPinned both take a
// WID — measured by calling them without one and getting "Cannot read
// properties of undefined (reading 'isNewsletter')", the fourth appearance of
// that shape in a day. The limit is checked BEFORE asking, so a caller is told
// why rather than watching the page refuse.
package chatstate

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
	stateBudget = 20 * time.Second
	stateTick   = 500 * time.Millisecond
)

var (
	// ErrState is the page refusing or throwing.
	ErrState = fmt.Errorf("chatstate: the page refused")
	// ErrNoChat is a conversation this account does not have.
	ErrNoChat = fmt.Errorf("chatstate: no such conversation")
	// ErrPinLimit is the account already holding as many pinned conversations
	// as it may. It is checked here rather than left to the page so the caller
	// learns WHY, with the numbers.
	ErrPinLimit = fmt.Errorf("chatstate: the pin limit is already reached")
	// ErrUnchanged is the postcondition: the call returned and the flag did not
	// move.
	ErrUnchanged = fmt.Errorf("chatstate: the page accepted the change and the conversation did not change")
)

// Change is what one state change did.
type Change struct {
	// Was is the value before, After the value once the page settled. A caller
	// that only got the new value could not tell "changed it" from "it was
	// already so", and those mean different things to a person looking at a
	// list.
	Was, After bool
	Waited     time.Duration
}

// Changed reports whether the call moved anything.
func (c Change) Changed() bool { return c.Was != c.After }

func (c Change) String() string {
	return fmt.Sprintf("chatstate.Change(was=%t after=%t changed=%t waited=%s)",
		c.Was, c.After, c.Changed(), c.Waited.Round(time.Millisecond))
}

// Setter changes conversation state on one session.
type Setter struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Setter.
func New(runner *engine.Runner, eval spa.Evaluator) *Setter {
	return &Setter{runner: runner, eval: eval}
}

// kind names the flag being set. It is a type rather than a string so a caller
// cannot ask for a field this package does not know how to verify.
type kind string

const (
	kindArchive kind = "archive"
	kindPin     kind = "pin"
)

// SetArchived archives or unarchives a conversation.
func (s *Setter) SetArchived(ctx context.Context, jid string, archived bool, label string) (Change, error) {
	return s.set(ctx, jid, kindArchive, archived, label)
}

// SetPinned pins or unpins a conversation.
//
// Pinning checks the account's limit first. Unpinning never can exceed one, so
// it is not checked — a guard that ran on the way out would refuse to undo what
// it just allowed.
func (s *Setter) SetPinned(ctx context.Context, jid string, pinned bool, label string) (Change, error) {
	return s.set(ctx, jid, kindPin, pinned, label)
}

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other, and each polled it
// until it stopped saying "pending" — so one could take the other's answer. The
// nonce comes from Go: a page-side Math.random or Date.now would put a decision
// and a clock where invariant 6 forbids them.
const stateKeyPrefix = "__headlessChatState"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

func (s *Setter) set(ctx context.Context, jid string, k kind, want bool, label string) (Change, error) {
	if strings.TrimSpace(jid) == "" {
		return Change{}, ErrNoChat
	}
	start := time.Now()

	key := nextStateKey()

	var kicked string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return s.eval(ctx, setScript(jid, k, want, key), &kicked)
	}); err != nil {
		return Change{}, fmt.Errorf("%w: %v", ErrState, err)
	}

	var out struct {
		Stage    string `json:"stage"`
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		Was      bool   `json:"was"`
		After    bool   `json:"after"`
		JID      string `json:"jid"`
		Pinned   int    `json:"pinned"`
		PinLimit int    `json:"pin_limit"`
	}
	deadline := time.Now().Add(stateBudget)
	for {
		var raw string
		if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return s.eval(ctx, resultScript(key), &raw)
		}); err != nil {
			return Change{}, fmt.Errorf("%w: %v", ErrState, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Change{}, fmt.Errorf("chatstate: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			// A CHAVE E' LIBERADA ao sair do laco (H177): sem isso a correcao
			// troca uma resposta cruzada por um global de pagina POR CHAMADA.
			var ignored string
			_ = s.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return s.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			break
		}
		if !time.Now().Before(deadline) {
			return Change{}, fmt.Errorf("%w: the page never settled within %s", ErrState, stateBudget)
		}
		time.Sleep(stateTick)
	}
	switch {
	case out.Why == "PIN_LIMIT":
		return Change{}, fmt.Errorf("%w: %d of %d already pinned", ErrPinLimit, out.Pinned, out.PinLimit)
	case out.Stage == "find" && !out.OK:
		return Change{}, fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
	case !out.OK:
		return Change{}, fmt.Errorf("%w at %s (%s)", ErrState, out.Stage, out.Why)
	}

	// THE POSTCONDITION. The page reports what the conversation became, read
	// from the same object it just changed, and this refuses the case that
	// matters: the call returned and the flag did not move.
	if out.After != want {
		return Change{}, fmt.Errorf("%w: %s is %t, wanted %t", ErrUnchanged, k, out.After, want)
	}
	return Change{Was: out.Was, After: out.After, Waited: time.Since(start)}, nil
}

// lookupExpr finds a conversation WITHOUT creating one. Changing the state of a
// conversation that does not exist is not a thing to do quietly.
const lookupExpr = `(async function (jidString) {
	const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
	const resolveIdentity = ` + spa.ResolveIdentityExpr + `;
	const r = await resolveIdentity(jidString);
	if (!r.ok) { return r; }
	// ChatCollection.get is NOT enough. It returns null for conversations that
	// plainly exist — which is why the send path falls through to
	// findOrCreateLatestChat, and why the first version of this lookup reported
	// NO_CHAT for the peer lab account the tests message every day.
	//
	// findExistingChat is the half of that pair which does NOT create, and
	// creating is exactly what changing a conversation's state must never do.
	let chat = Chats.get(r.wid);
	if (!chat) {
		const Find = window.require('` + string(spa.ModuleFindChatAction) + `');
		chat = await Find.findExistingChat(r.wid);
	}
	if (!chat) { return { ok: false, why: 'NO_CHAT' }; }
	return { ok: true, wid: r.wid, chat: chat };
})`

func setScript(jid string, k kind, want bool, key string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(key) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(key) + `] = v; };
		const lookup = ` + lookupExpr + `;
		(async () => {
		let stage = 'find';
		try {
			const r = await lookup(` + strconv.Quote(jid) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }
			const chat = r.chat;
			const want = ` + strconv.FormatBool(want) + `;
			const which = ` + strconv.Quote(string(k)) + `;
			const was = which === 'archive' ? !!chat.archive : !!chat.pin;

			// A REDUNDANT REQUEST IS REFUSED BY THE APP, and that is measured
			// rather than assumed. In one controlled run against the live
			// account: asking for the OPPOSITE of the current state was
			// ACCEPTED, and asking for the state the conversation already had
			// threw ActionError("Could not perform action.").
			//
			// So asking anyway is asking the app to do nothing and be told off
			// for it. Nothing to change is a successful no-op, the same shape
			// as a conversation with nothing unread (H52).
			if (was === want) {
				park({ stage: 'done', ok: true, why: '', was: was, after: was, jid: r.jid });
				return;
			}

			stage = 'apply';
			if (which === 'archive') {
				const A = window.require('` + string(spa.ModuleSetArchiveChatAction) + `');
				// TWO ARGUMENTS ARE ENOUGH. The arity is 3, and the third being
				// missing was my first theory for the refusal — it was wrong:
				// the refusal is about asking for a state the conversation
				// already has, which the guard above now prevents.
				await A.setArchive(chat, want);
			} else {
				// THE LIMIT, checked before asking. Both helpers take a WID, and
				// calling them without one throws on isNewsletter.
				if (want) {
					try {
						const B = window.require('` + string(spa.ModuleChatPinBridge) + `');
						const limit = B.getPinLimit(r.wid);
						const used = await B.getNumConversationsPinned(r.wid);
						if (typeof limit === 'number' && typeof used === 'number' && used >= limit && !was) {
							park({ stage, ok: false, why: 'PIN_LIMIT', was: was,
								pinned: used, pin_limit: limit });
							return;
						}
					} catch (e) { /* the limit is advisory here; the page still decides */ }
				}
				const P = window.require('` + string(spa.ModuleSetPinChatAction) + `');
				await P.setPin(chat, want);
			}
			// THE AFTER VALUE, READ FROM THE CHAT THIS SCRIPT ALREADY HOLDS.
			//
			// A separate polling read was tried first and fought back: get()
			// answers null for conversations that exist, findExistingChat is
			// async so the answer had to be parked, and re-kicking that read on
			// every turn reset the parked answer before it landed. Three
			// defects, all in plumbing for a value that was already here.
			//
			// Measured: the flag is updated by the time setArchive resolves —
			// the probe read it as true immediately after the await. So the
			// object in hand is the answer, and no second lookup is needed.
			const after = which === 'archive' ? !!chat.archive : !!chat.pin;
			park({ stage: 'done', ok: true, why: '', was: was, after: after, jid: r.jid });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

func resultScript(key string) string {
	return `JSON.stringify((() => {
	const s = window[` + strconv.Quote(key) + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
}
