// Package block blocks and unblocks a contact.
//
// THE TWO CALLS HAVE DIFFERENT SHAPES, read from each function's own
// toString() against the live build (probe_block_test.go):
//
//	blockContact({bizOptOutArgs, blockEntryPoint, contact, skipCtwa1pdNbfSignal})
//	unblockContact(contact, blockEntryPoint)
//
// Assuming the second matched the first is precisely the assumption that cost
// H58 a live failure on addParticipantsJob/removeParticipantsJob. Two sibling
// pairs, both asymmetric, is enough to stop treating it as bad luck.
//
// THE APP THROWS ON A PRECONDITION, and the message is in the bundle:
//
//	"[blocklist] trying to block a pn contact (id: …) without a chat"
//
// So blocking a PHONE contact requires an existing chat. This package checks
// that itself and returns ErrNoChatToBlockFrom, because a caller can act on
// "there is no conversation with this person" and cannot act on a thrown
// string from someone else's telemetry path.
//
// WHAT IT REPORTS: counts. A blocklist is a list of people, and this package
// never renders one — Result carries how many entries there were before and
// after, which is what proves the act without naming anyone.
package block

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
	blockBudget = 30 * time.Second
	blockTick   = 500 * time.Millisecond
)

// entryPoint is the app's vocabulary for where a block came from. "chat" is the
// measured value for an ordinary conversation; the app maps it to a telemetry
// metric before doing anything, so an invented string walks into that mapping.
const entryPoint = "chat"

var (
	// ErrBlock is the page refusing or throwing.
	ErrBlock = fmt.Errorf("block: the page refused")
	// ErrNoContact is an identity that is not in the roster.
	ErrNoContact = fmt.Errorf("block: no such contact in the loaded roster")
	// ErrNotOnWhatsApp is the identity failing to resolve.
	ErrNotOnWhatsApp = fmt.Errorf("block: that identity does not resolve on this build")
	// ErrGroup is a group jid. A group is not a contact and cannot be blocked;
	// leaving it is the act a caller actually wants, and it is not this one.
	ErrGroup = fmt.Errorf("block: a group cannot be blocked")
	// ErrNoChatToBlockFrom is the app's own precondition, checked here instead
	// of thrown there.
	ErrNoChatToBlockFrom = fmt.Errorf("block: this build refuses to block a phone contact with no existing conversation")
	// ErrBlocklistUnchanged is the postcondition: the call returned and the
	// blocklist says otherwise.
	ErrBlocklistUnchanged = fmt.Errorf("block: the page accepted the change and the blocklist did not move")
)

// Result is what a block or unblock did, in counts.
type Result struct {
	// Before and After are blocklist SIZES. Never the entries.
	Before, After int
	// AlreadyInState is true when there was nothing to do — blocking somebody
	// already blocked. It is a success, not a failure, for the same reason an
	// already-archived chat is (H55): the caller's intent is satisfied.
	AlreadyInState bool
	// Waited is how long the page took to reflect it.
	Waited time.Duration
}

// Changed reports whether the blocklist actually moved.
func (r Result) Changed() bool { return r.Before != r.After }

func (r Result) String() string {
	return fmt.Sprintf("block.Result(before=%d after=%d already=%t waited=%s)",
		r.Before, r.After, r.AlreadyInState, r.Waited.Round(time.Millisecond))
}

// Blocker blocks and unblocks contacts on one session.
type Blocker struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Blocker.
func New(runner *engine.Runner, eval spa.Evaluator) *Blocker {
	return &Blocker{runner: runner, eval: eval}
}

const stateKey = "__waHeadlessBlock"

// Block stops a contact from reaching this account.
func (b *Blocker) Block(ctx context.Context, jid, label string) (Result, error) {
	return b.set(ctx, jid, true, label)
}

// Unblock reverses it.
func (b *Blocker) Unblock(ctx context.Context, jid, label string) (Result, error) {
	return b.set(ctx, jid, false, label)
}

func (b *Blocker) set(ctx context.Context, jid string, want bool, label string) (Result, error) {
	if strings.TrimSpace(jid) == "" {
		return Result{}, ErrNoContact
	}
	if strings.HasSuffix(jid, "@g.us") {
		return Result{}, ErrGroup
	}
	start := time.Now()

	var kicked string
	if err := b.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return b.eval(ctx, blockScript(jid, want), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrBlock, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		Before  int    `json:"before"`
		After   int    `json:"after"`
		Already bool   `json:"already"`
	}
	deadline := time.Now().Add(blockBudget)
	for {
		var raw string
		if err := b.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return b.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrBlock, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("block: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrBlock, blockBudget)
		}
		time.Sleep(blockTick)
	}

	switch {
	case out.Why == "NOT_ON_WHATSAPP" || out.Why == "WID_NULL":
		return Result{}, ErrNotOnWhatsApp
	case out.Why == "NO_CONTACT":
		return Result{}, ErrNoContact
	case out.Why == "NO_CHAT":
		return Result{}, ErrNoChatToBlockFrom
	case out.Why == "IS_GROUP":
		return Result{}, ErrGroup
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrBlock, out.Stage, out.Why)
	}

	res := Result{Before: out.Before, After: out.After, AlreadyInState: out.Already, Waited: time.Since(start)}
	// THE POSTCONDITION. A blocklist that did not move after a call that was
	// supposed to move it is the silent success this module exists to refuse —
	// and here it matters more than usual, because a caller who believes
	// somebody is blocked stops watching for them.
	if !out.Already && !res.Changed() {
		return Result{}, fmt.Errorf("%w (before=%d after=%d)", ErrBlocklistUnchanged, out.Before, out.After)
	}
	return res, nil
}

func blockScript(jid string, want bool) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(stateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(stateKey) + `] = v; };
		(async () => {
		let stage = 'resolve';
		try {
			const resolved = await (` + spa.ResolveIdentityExpr + `)(` + strconv.Quote(jid) + `);
			if (!resolved.ok) { park({ stage, ok: false, why: resolved.why }); return; }
			if (resolved.isGroup) { park({ stage, ok: false, why: 'IS_GROUP' }); return; }

			stage = 'contact';
			// THE MODEL, not the id. Both calls read .id off their argument and
			// unproxy it; passing the wid produces the same
			// "reading '<field>' of undefined" this module has now paid for
			// five times.
			const CC = window.require('` + string(spa.ModuleContactCollection) + `').ContactCollection;
			const contact = CC.get(resolved.wid) || CC.get(resolved.jid);
			if (!contact) { park({ stage, ok: false, why: 'NO_CONTACT' }); return; }

			const BL = window.require('` + string(spa.ModuleBlocklistCollection) + `').BlocklistCollection;
			const before = BL.getModelsArray().length;
			const wasBlocked = !!contact.isContactBlocked;
			const want = ` + strconv.FormatBool(want) + `;
			if (wasBlocked === want) {
				park({ stage: 'done', ok: true, why: 'ALREADY', already: true,
					before: before, after: before });
				return;
			}

			stage = 'apply';
			const A = window.require('` + string(spa.ModuleBlockContactAction) + `');
			if (want) {
				// The app throws for a phone contact with no chat, and its own
				// message says so. Refusing here turns a thrown string into an
				// answer the caller can act on.
				const chat = window.require('` + string(spa.ModuleChatCollection) + `')
					.ChatCollection.get(contact.id);
				if (!chat && contact.id && contact.id.server === 'c.us') {
					park({ stage, ok: false, why: 'NO_CHAT' }); return;
				}
				// ONE OBJECT — measured, and different from its sibling.
				await A.blockContact({ contact: contact, blockEntryPoint: ` + strconv.Quote(entryPoint) + ` });
			} else {
				// TWO POSITIONAL — measured. Same module, other shape.
				await A.unblockContact(contact, ` + strconv.Quote(entryPoint) + `);
			}

			stage = 'verify';
			park({ stage: 'done', ok: true, why: '', already: false,
				before: before, after: BL.getModelsArray().length });
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

// List reports WHO is blocked, which this package could count and not name.
//
// THE SIZE WAS ALWAYS HERE AND THE NAMES WERE NOT. Result carries Before and
// After as blocklist SIZES with an explicit "never the entries" — a deliberate
// narrowing when the only question was "did the change land". The ledger row for
// getBlockedContacts sat at PARTIAL saying listing "não é exposto", which was
// true about this surface and never checked against the page.
//
// The page does expose it: WAWebCollections.Blocklist answers getModelsArray,
// measured 0 on this account because nobody is blocked (H146). What does NOT
// exist is the path a caller might reach for instead — of 945 contacts, ZERO
// carry an isBlocked field, so filtering the roster would silently return
// nothing forever.
//
// IT RETURNS IDENTITIES, and that is a widening of what this package emits. It
// is the point of the method: a count cannot tell a caller whom to unblock.
func (b *Blocker) List(ctx context.Context, label string) ([]string, error) {
	var raw string
	if err := b.runner.Do(ctx, engine.OpStateProbe, label+"/list", func(ctx context.Context) error {
		return b.eval(ctx, listScript, &raw)
	}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlock, err)
	}
	var out struct {
		OK      bool     `json:"ok"`
		Why     string   `json:"why"`
		Blocked []string `json:"blocked"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("block: unexpected answer: %w", err)
	}
	if !out.OK {
		return nil, fmt.Errorf("%w (%s)", ErrBlock, out.Why)
	}
	return out.Blocked, nil
}

// listScript reads the blocklist's entries.
//
// A BLOCKLIST MODEL IS NOT A CONTACT. It is keyed by identity, so the id is read
// the same way every other reader in this module reads one — through
// _serialized when present — rather than assuming the model shape.
const listScript = `JSON.stringify((() => {
	try {
		const BL = window.require('` + string(spa.ModuleBlocklistCollection) + `').BlocklistCollection;
		const all = (typeof BL.getModelsArray === 'function') ? BL.getModelsArray() : [];
		const jid = v => (v && v._serialized) ? v._serialized : (typeof v === 'string' ? v : '');
		const blocked = [];
		for (const m of all) {
			const s = jid(m && m.id);
			if (s) { blocked.push(s); }
		}
		return { ok: true, why: '', blocked: blocked };
	} catch (e) {
		return { ok: false, why: String((e && e.message) || e).slice(0, 140), blocked: [] };
	}
})())`
