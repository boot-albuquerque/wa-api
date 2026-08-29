// Package revoke deletes a message for everyone.
//
// IT IS THE FIRST DESTRUCTIVE CAPABILITY IN THIS MODULE, and it is built to say
// no more readily than the others. Everything else here adds or changes
// something recoverable; this removes a message from other people's phones, and
// there is no undo.
//
// THE CALL SHAPE CAME FROM THE APP'S OWN CODE, and the first argument is a
// RECORD rather than the message:
//
//	sendRevoke({type: 'message', data: msg}, revokeType, clearMedia)
//
// revokeType is WAWebCmd.Revoke.Sender or .Admin, and the app chooses between
// them with WAWebMsgActionCapability.canSenderRevokeMsg. Consulting that is not
// politeness: revoking as Sender when the account is only an Admin — or the
// reverse — is asking for something it is not entitled to.
//
// WHAT IT REFUSES BEFORE ASKING:
//
//	a message that is not loaded          nothing to revoke
//	a message the page says cannot be     WhatsApp has a time window, and
//	  revoked                             asking outside it is asking to fail
package revoke

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
	revokeBudget = 30 * time.Second
	revokeTick   = 500 * time.Millisecond
)

var (
	// ErrRevoke is the page refusing or throwing.
	ErrRevoke = fmt.Errorf("revoke: the page refused")
	// ErrNoMessage is an id that is not in the loaded collection.
	ErrNoMessage = fmt.Errorf("revoke: no such message in the loaded collection")
	// ErrNotRevocable is the page saying this message cannot be deleted for
	// everyone — usually the time window has passed. It is its own error
	// because a caller can do something about it: delete it locally instead.
	ErrNotRevocable = fmt.Errorf("revoke: this message can no longer be deleted for everyone")
	// ErrStillThere is the postcondition: the call returned and the message is
	// not revoked.
	ErrStillThere = fmt.Errorf("revoke: the page accepted the deletion and the message is still there")
	// ErrStillLoaded is ForMe's postcondition. It is a DIFFERENT check from
	// ErrStillThere and not a rename: a revoke leaves the message in place
	// wearing a revoked mark, while a local delete must leave the collection
	// entirely. Asking the revoked question of a local delete would pass on a
	// message that never moved.
	ErrStillLoaded = fmt.Errorf("revoke: the page accepted the local deletion and the message is still loaded")
)

// Result is what a revocation did.
type Result struct {
	// As says which entitlement the page used — the account's own message, or
	// an admin removing somebody else's. It is reported because they are
	// different acts and a caller should know which one happened.
	As string
	// Waited is how long the page took to reflect it.
	Waited time.Duration
}

func (r Result) String() string {
	return fmt.Sprintf("revoke.Result(as=%s waited=%s)", r.As, r.Waited.Round(time.Millisecond))
}

// Revoker deletes messages for everyone on one session.
type Revoker struct {
	runner *engine.Runner
	eval   spa.Evaluator
}

// New builds a Revoker.
func New(runner *engine.Runner, eval spa.Evaluator) *Revoker {
	return &Revoker{runner: runner, eval: eval}
}

// stateKeyPrefix is a PREFIX, not a key (H177). One shared page global meant two
// concurrent calls on the same session overwrote each other, and each polled it
// until it stopped saying "pending" — so one could take the other's answer. The
// nonce comes from Go: a page-side Math.random or Date.now would put a decision
// and a clock where invariant 6 forbids them.
const stateKeyPrefix = "__headlessRevoke"

var stateKeySeq atomic.Uint64

func nextStateKey() string {
	return stateKeyPrefix + "_" + strconv.FormatUint(stateKeySeq.Add(1), 10)
}

// ForEveryone deletes a message from every participant's phone.
//
// clearMedia decides whether the local media copy goes too. It is a parameter
// rather than a constant because the two are genuinely different intentions,
// and defaulting to destroying more than asked is not a default this package
// will make on someone's behalf.
func (r *Revoker) ForEveryone(ctx context.Context, msgID string, clearMedia bool, label string) (Result, error) {
	if strings.TrimSpace(msgID) == "" {
		return Result{}, ErrNoMessage
	}
	start := time.Now()

	key := nextStateKey()

	var kicked string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return r.eval(ctx, revokeScript(msgID, clearMedia, key), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrRevoke, err)
	}

	var out struct {
		Stage   string `json:"stage"`
		OK      bool   `json:"ok"`
		Why     string `json:"why"`
		As      string `json:"as"`
		Revoked bool   `json:"revoked"`
	}
	deadline := time.Now().Add(revokeBudget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return r.eval(ctx, resultScript(key), &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrRevoke, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("revoke: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			// A CHAVE E' LIBERADA ao sair do laco (H177): sem isso a correcao
			// troca uma resposta cruzada por um global de pagina POR CHAMADA.
			var ignored string
			_ = r.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return r.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			break
		}
		if !time.Now().Before(deadline) {
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrRevoke, revokeBudget)
		}
		time.Sleep(revokeTick)
	}
	switch {
	case out.Why == "NOT_REVOCABLE":
		return Result{}, ErrNotRevocable
	case out.Stage == "find" && !out.OK:
		return Result{}, fmt.Errorf("%w (%s)", ErrNoMessage, out.Why)
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrRevoke, out.Stage, out.Why)
	}
	// THE POSTCONDITION. Deleting for everyone is the one act here that cannot
	// be undone, so reporting it without checking would be the worst kind of
	// silent success: the caller believes a message is gone from other people's
	// phones when it is not.
	if !out.Revoked {
		return Result{}, ErrStillThere
	}
	return Result{As: out.As, Waited: time.Since(start)}, nil
}

func revokeScript(msgID string, clearMedia bool, key string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(key) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(key) + `] = v; };
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

			// WHO IS ENTITLED, asked of the page. WhatsApp has a time window
			// for deleting your own message, and an admin path for somebody
			// else's; picking the wrong one is asking for something this
			// account may not have.
			const Cap = window.require('` + string(spa.ModuleMsgActionCapability) + `');
			const Cmd = window.require('` + string(spa.ModuleCmd) + `');
			const asSender = !!(Cap.canSenderRevokeMsg && Cap.canSenderRevokeMsg(msg));
			const asAdmin = !!(Cap.canAdminRevokeMsg && Cap.canAdminRevokeMsg(msg));
			if (!asSender && !asAdmin) {
				park({ stage, ok: false, why: 'NOT_REVOCABLE' });
				return;
			}
			const kind = asSender ? Cmd.Revoke.Sender : Cmd.Revoke.Admin;

			stage = 'revoke';
			const A = window.require('` + string(spa.ModuleRevokeMsgAction) + `');
			// A RECORD, not the message. Read from the app's own call:
			// sendRevoke({type:'message', data: msg}, kind, clearMedia).
			await A.sendRevoke({ type: 'message', data: msg }, kind, ` + strconv.FormatBool(clearMedia) + `);

			stage = 'verify';
			park({ stage: 'done', ok: true, why: '',
				as: asSender ? 'sender' : 'admin',
				revoked: !!(msg.isRevokedMsg || msg.type === 'revoked' || msg.revokeSender) });
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
	if (!s) { return { stage: 'revoke', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
}

// ForMe deletes a message from THIS DEVICE only.
//
// IT IS THE ANSWER ErrNotRevocable ALREADY POINTED AT. That error says the
// window for deleting for everyone has closed and suggests deleting locally
// instead, and until now this package named a remedy it did not offer.
//
// THE TWO ARE NOT DEGREES OF THE SAME ACT. ForEveryone changes what other people
// see and is announced to them; this changes nothing for anyone else and is
// invisible outside this session. That is why it does not ask
// canSenderRevokeMsg: there is no entitlement to check, because nobody else is
// affected — and running the capability check here would refuse, on somebody
// else's behalf, an act that only touches this account's own store.
//
// The call shape is the reference's modern branch, confirmed against this build
// rather than assumed: WAWebCmd.Cmd.sendDeleteMsgs exists and takes the record
// form, sendDeleteMsgs(chat, {list, type}, clearMedia).
func (r *Revoker) ForMe(ctx context.Context, msgID string, clearMedia bool, label string) (Result, error) {
	if strings.TrimSpace(msgID) == "" {
		return Result{}, ErrNoMessage
	}
	start := time.Now()

	key := nextStateKey()

	var kicked string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return r.eval(ctx, deleteScript(msgID, clearMedia, key), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrRevoke, err)
	}

	var out struct {
		Stage  string `json:"stage"`
		OK     bool   `json:"ok"`
		Why    string `json:"why"`
		Loaded bool   `json:"loaded"`
	}
	deadline := time.Now().Add(revokeBudget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return r.eval(ctx, resultScript(key), &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrRevoke, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("revoke: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
			// A CHAVE E' LIBERADA ao sair do laco (H177): sem isso a correcao
			// troca uma resposta cruzada por um global de pagina POR CHAMADA.
			var ignored string
			_ = r.runner.Do(ctx, engine.OpStateProbe, label+"/release", func(c context.Context) error {
				return r.eval(c, `(() => { try { delete window.`+key+`; } catch (e) { window.`+key+` = null; } return "ok"; })()`, &ignored)
			})
			break
		}
		if !time.Now().Before(deadline) {
			return Result{}, fmt.Errorf("%w: the page never settled within %s", ErrRevoke, revokeBudget)
		}
		time.Sleep(revokeTick)
	}
	switch {
	case out.Stage == "find" && !out.OK:
		return Result{}, fmt.Errorf("%w (%s)", ErrNoMessage, out.Why)
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrRevoke, out.Stage, out.Why)
	}
	// THE POSTCONDITION IS WAITED FOR FROM HERE, and that was measured rather
	// than assumed. The first version verified inside the page immediately after
	// the await, and it failed against production on a message sent seconds
	// earlier: sendDeleteMsgs resolves BEFORE the collection lets the model go.
	//
	// The fix is not a sleep in the page — invariant 6 keeps the clock on this
	// side — it is this loop. Which is also why the script no longer answers
	// `loaded` at all: a page that reported the postcondition would be reporting
	// it at the one instant it is guaranteed to be wrong.
	if err := r.waitGone(ctx, msgID, label); err != nil {
		return Result{}, err
	}
	return Result{As: asLocal, Waited: time.Since(start)}, nil
}

// waitGone polls the collection until the message is not in it.
func (r *Revoker) waitGone(ctx context.Context, msgID, label string) error {
	deadline := time.Now().Add(revokeBudget)
	for {
		var raw string
		if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/gone", func(ctx context.Context) error {
			return r.eval(ctx, loadedScript(msgID), &raw)
		}); err != nil {
			return fmt.Errorf("%w: %v", ErrRevoke, err)
		}
		var out struct {
			Loaded bool `json:"loaded"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return fmt.Errorf("revoke: unexpected answer: %w", err)
		}
		if !out.Loaded {
			return nil
		}
		if !time.Now().Before(deadline) {
			return ErrStillLoaded
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(revokeTick):
		}
	}
}

// loadedScript asks whether the collection still holds a message.
//
// IT RE-READS THE COLLECTION rather than consulting a model. A removed model
// stays a perfectly valid object in whatever variable holds it, so asking the
// model whether it is gone answers "I am right here" forever.
func loadedScript(msgID string) string {
	return `JSON.stringify((() => {
		try {
			const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			const want = ` + strconv.Quote(msgID) + `;
			for (const m of coll.getModelsArray()) {
				try { if (m.id && m.id.id === want) { return { loaded: true }; } } catch (e) {}
			}
			return { loaded: false };
		} catch (e) { return { loaded: true }; }
	})())`
}

// asLocal names the entitlement ForMe used, which is none: the act touches only
// this account's own store. It is a constant rather than a literal because As is
// compared by callers and two spellings would be two bugs.
const asLocal = "local"

func deleteScript(msgID string, clearMedia bool, key string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(key) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(key) + `] = v; };
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

			// O CHAT E' EXIGIDO PELA CHAMADA, e vem do proprio id da mensagem. Um
			// apagamento local pedido sem chat nao tem de onde remover.
			const CC = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
			const remote = msg.id && msg.id.remote;
			const chat = CC.get(remote && remote._serialized ? remote._serialized : remote);
			if (!chat) { park({ stage, ok: false, why: 'NO_CHAT' }); return; }

			stage = 'delete';
			const Cmd = window.require('` + string(spa.ModuleCmd) + `').Cmd;
			await Cmd.sendDeleteMsgs(chat, { list: [msg], type: 'message' }, ` + strconv.FormatBool(clearMedia) + `);

			park({ stage: 'done', ok: true, why: '' });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}
