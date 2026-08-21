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
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
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

const stateKey = "__waHeadlessRevoke"

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

	var kicked string
	if err := r.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return r.eval(ctx, revokeScript(msgID, clearMedia), &kicked)
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
			return r.eval(ctx, resultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrRevoke, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("revoke: unexpected answer: %w", err)
		}
		if out.Stage != "pending" {
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

func revokeScript(msgID string, clearMedia bool) string {
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

const resultScript = `JSON.stringify((() => {
	const s = window[` + `"` + stateKey + `"` + `];
	if (!s) { return { stage: 'revoke', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
