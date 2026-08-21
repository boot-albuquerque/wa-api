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
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
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

const stateKey = "__waHeadlessChatState"

func (s *Setter) set(ctx context.Context, jid string, k kind, want bool, label string) (Change, error) {
	if strings.TrimSpace(jid) == "" {
		return Change{}, ErrNoChat
	}
	start := time.Now()

	var kicked string
	if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return s.eval(ctx, setScript(jid, k, want), &kicked)
	}); err != nil {
		return Change{}, fmt.Errorf("%w: %v", ErrState, err)
	}

	var out struct {
		Stage    string `json:"stage"`
		OK       bool   `json:"ok"`
		Why      string `json:"why"`
		Was      bool   `json:"was"`
		JID      string `json:"jid"`
		Pinned   int    `json:"pinned"`
		PinLimit int    `json:"pin_limit"`
	}
	deadline := time.Now().Add(stateBudget)
	for {
		var raw string
		if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return s.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Change{}, fmt.Errorf("%w: %v", ErrState, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Change{}, fmt.Errorf("chatstate: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
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

	// THE POSTCONDITION, POLLED. Reacting taught that page state lands a moment
	// after the call resolves (H53), so reading once here would be a race that
	// passes today and fails tomorrow.
	// THE RESOLVED jid, not the caller's. The first version verified with
	// createWid(jid) on whatever was passed in, which finds nothing when the
	// caller typed a phone number: this build files chats under the identity
	// the server assigns (H34, H39). The set path resolved and the read path
	// did not — the same split that broke group sending (H48), made by me again
	// in the same file.
	verifyJID := out.JID
	if verifyJID == "" {
		verifyJID = jid
	}
	after, err := s.waitFor(ctx, verifyJID, k, want, label)
	if err != nil {
		return Change{}, err
	}
	return Change{Was: out.Was, After: after, Waited: time.Since(start)}, nil
}

func (s *Setter) waitFor(ctx context.Context, jid string, k kind, want bool, label string) (bool, error) {
	deadline := time.Now().Add(stateBudget)
	last := !want
	for {
		var raw string
		if err := s.runner.Do(ctx, engine.OpStateProbe, label+"/verify", func(ctx context.Context) error {
			return s.eval(ctx, readScript(jid, k), &raw)
		}); err != nil {
			return false, fmt.Errorf("%w: %v", ErrState, err)
		}
		var got struct {
			Found bool `json:"found"`
			Value bool `json:"value"`
		}
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			return false, fmt.Errorf("chatstate: unexpected verify answer: %w", err)
		}
		if !got.Found {
			return false, ErrNoChat
		}
		last = got.Value
		if got.Value == want {
			return got.Value, nil
		}
		if !time.Now().Before(deadline) {
			return last, fmt.Errorf("%w: %s is %t, wanted %t", ErrUnchanged, k, last, want)
		}
		time.Sleep(stateTick)
	}
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

func setScript(jid string, k kind, want bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
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

			stage = 'apply';
			if (which === 'archive') {
				const A = window.require('` + string(spa.ModuleSetArchiveChatAction) + `');
				// THE THIRD ARGUMENT. setArchive has arity 3 and passing two
				// produced "Could not perform action." — the app's own refusal,
				// not a crash. The third is passed as true, which is what a
				// user-initiated archive is.
				await A.setArchive(chat, want, true);
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
			park({ stage: 'done', ok: true, why: '', was: was, jid: r.jid });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

func readScript(jid string, k kind) string {
	return `JSON.stringify((() => {
		const Chats = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
		const WF = window.require('` + string(spa.ModuleWidFactory) + `');
		const which = ` + strconv.Quote(string(k)) + `;
		// The jid here is the RESOLVED one, handed down from the set call, so
		// createWid is enough and no second round trip to the server happens on
		// every poll.
		const wid = WF.createWid(` + strconv.Quote(jid) + `);
		let chat = wid ? Chats.get(wid) : null;
		if (!chat && wid) {
			// Same reason as the lookup: get() answers null for conversations
			// that exist. This path must not create one either.
			try { chat = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection.get(wid); } catch (e) {}
		}
		if (!chat) { return { found: false, value: false }; }
		return { found: true, value: which === 'archive' ? !!chat.archive : !!chat.pin };
	})())`
}

const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'apply', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
