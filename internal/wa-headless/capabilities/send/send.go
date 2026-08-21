// Package send dispatches a text message through the real SPA.
//
// IT VERIFIES THE POSTCONDITION, and that is the capability rather than an
// extra. Invariant 14 says sendText throws on failure and never returns silent
// success, and item 11 of this initiative's briefing says a driver call
// returning nil is not the WhatsApp operation having happened. A send that only
// checked "the page did not throw" would report success for a message the
// server never accepted — and the caller would find out from a user, later.
//
// THE SURFACE WAS MEASURED, not copied. On 2026-08-20 against this build:
//
//	WAWebSendTextMsgChatAction  sendTextMsgToChat/3, addAndSendTextMsg/3
//	WAWebWidFactory             asChatWid/1, asUserWidOrThrow/1
//	WAWebChatCollection         find, get, add, getModelsArray
//
// Four names guessed from other builds — WAWebSendMsg, WAWebMsgSend,
// WAWebSendMessage, WAWebComposeMessage — do not exist here. Copying the
// reference implementation's module list would have failed on all four.
package send

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-api/internal/wa-headless/capabilities/messagemeta"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// Verification bounds. Sending is fast; the message appearing in the local
// collection is what takes a moment.
// These two are var, not const, for ONE reason: the tests compress the clock.
// Production never assigns them, and a 20-second budget inside a unit suite
// would buy nothing — the failure paths are what need exercising, and each one
// would otherwise sit out the full wait.
var (
	verifyBudget = 20 * time.Second
	verifyTick   = 500 * time.Millisecond
)

const (
	// clockSkewAllowance widens the "is this message mine and new" window,
	// because the timestamp comes from the server and not from this host.
	clockSkewAllowance = 2 * time.Minute
)

var (
	// ErrNoChat is a recipient the page could not resolve to a chat.
	ErrNoChat = fmt.Errorf("send: the recipient did not resolve to a chat")
	// ErrDispatch is the send call itself failing or throwing.
	ErrDispatch = fmt.Errorf("send: the page refused the send")
	// ErrUnverified is the one that matters: the send call returned without
	// error and NO outgoing message appeared. The page said yes and nothing
	// happened, which is precisely the silent success invariant 14 forbids.
	ErrUnverified = fmt.Errorf("send: dispatched but no outgoing message appeared")
	// ErrNeverLeft is a message that appeared in this session and never reached
	// the server.
	//
	// It is distinct from ErrUnverified because the repairs differ: nothing
	// appearing is a dispatch that did not happen, and something appearing at
	// ack 0 is a dispatch the application accepted and the socket did not
	// carry. The second is what a poll does on this build, every time (H98).
	ErrNeverLeft = fmt.Errorf("send: the message appeared locally and never reached the server")
)

// Result is a message this session sent, identified the same way every other
// capability identifies one.
type Result struct {
	ID        messagemeta.MessageID
	Timestamp time.Time
	// Ack is the delivery state reached before this call returned: 1 server,
	// 2 device, 3 read. It is never 0 on a successful return — a message still
	// pending is reported as ErrNeverLeft rather than as a send.
	Ack int
	// Waited is how long verification took. A number worth having: it is the
	// difference between "the send is confirmed" and "the send is confirmed
	// eventually", and only measurement tells them apart.
	Waited time.Duration
}

// String redacts, like every other rendering in this module.
func (r Result) String() string {
	return fmt.Sprintf("send.Result(id=%s at=%s ack=%d waited=%s)",
		r.ID.ID, r.Timestamp.UTC().Format(time.RFC3339), r.Ack, r.Waited.Round(time.Millisecond))
}

// Text sends text to a chat and returns only after proving it was sent.
//
// The four steps are separated on purpose, and each failure names WHICH one
// broke: a recipient that does not resolve, a page that refuses, and a
// dispatch that leaves no trace are three different problems with three
// different repairs, and one generic "send failed" would hide that.
func Text(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	toJID, text, label string) (Result, error) {

	if strings.TrimSpace(toJID) == "" {
		return Result{}, fmt.Errorf("send: empty recipient")
	}
	if text == "" {
		// An empty send would "succeed" while producing nothing, which is the
		// silent success this package exists to refuse.
		return Result{}, fmt.Errorf("send: empty text")
	}

	// RESOLVE + ACT, in the STORE-AND-POLL shape.
	//
	// Not one call, because engine.Tab.Evaluate does NOT await promises: an
	// async page function is stringified as a Promise, which decodes to an
	// empty object and reads exactly like a dispatch that failed with no
	// reason. That is what the first version did, and the symptom — "refused
	// the send at  ()" with both fields empty — is what gave it away.
	//
	// So the page starts the work, parks the outcome on a named global, and Go
	// polls for it. Same shape as the message subscription, and for the same
	// reason: the clock stays on this side.
	sentAt := time.Now().Add(-clockSkewAllowance)
	var kicked string //nolint // o valor é ignorado: o resultado vem do poll
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/dispatch", func(ctx context.Context) error {
		return eval(ctx, dispatchScript(toJID, text), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrDispatch, err)
	}

	var out struct {
		Stage string `json:"stage"`
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		// JID is the identity the SERVER returned, not the one the caller
		// passed. They differ on this build, and verification needs the
		// server's — see the note where it is parked.
		JID string `json:"jid"`
	}
	dispatchDeadline := time.Now().Add(verifyBudget)
	for {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, label+"/dispatch-result", func(ctx context.Context) error {
			return eval(ctx, dispatchResultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrDispatch, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("send: unexpected dispatch answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if time.Now().Before(dispatchDeadline) {
			time.Sleep(verifyTick)
			continue
		}
		return Result{}, fmt.Errorf("%w: the page never settled the dispatch within %s",
			ErrDispatch, verifyBudget)
	}
	switch {
	case out.Stage == "resolve" && !out.OK:
		return Result{}, fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrDispatch, out.Stage, out.Why)
	}

	// VERIFY. Nothing above proves the message exists — the page accepting a
	// call is not the account having sent anything.
	if out.JID == "" {
		return Result{}, fmt.Errorf("%w: the page reported success without a resolved identity", ErrDispatch)
	}
	res, err := verify(ctx, runner, eval, out.JID, sentAt, label, kindText)
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// sendStateKey is where the page parks the dispatch outcome.
const sendStateKey = "__waHeadlessSendResult"

// dispatchResultScript reads the parked outcome. "pending" means the page is
// still working — distinct from a failure, because a caller that treated
// "not finished" as "failed" would retry a send that is about to succeed.
const dispatchResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + sendStateKey + `"` + `];
	if (!s) { return { stage: 'dispatch', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`

// resolveChatExpr is the identity-and-chat half of a send, SHARED by every
// kind of message rather than copied per kind.
//
// It exists because the text path paid four measured corrections to get this
// sequence right (H34), and a second copy would be a second place for those
// four to be re-learned. It is a JavaScript function of (jidString) returning
// {ok, why, wid, jid, chat}.
//
// Nothing here is optional and nothing is decoration: each step exists because
// a simpler one was measured failing.
const resolveChatExpr = `(async function (jidString) {
	const ChatCollection = window.require('` + string(spa.ModuleChatCollection) + `').ChatCollection;
	const resolveIdentity = ` + spa.ResolveIdentityExpr + `;
	const r = await resolveIdentity(jidString);
	if (!r.ok) { return r; }

	// A chat that does not exist yet is the ORDINARY case for a first message,
	// so obtaining one cannot be a lookup. Measured: get() returns null for a
	// correspondent never spoken to, and ChatCollection.find throws
	// "this.findImpl is not a function". findOrCreateLatestChat works either
	// way, and it is looked up by the SERVER'S wid, not the phone one.
	//
	// THIS IS THE HALF PRESENCE MUST NOT HAVE: announcing that you are typing
	// may not bring a conversation into existence.
	let chat = ChatCollection.get(r.wid);
	if (!chat) {
		const Find = window.require('` + string(spa.ModuleFindChatAction) + `');
		chat = await Find.findOrCreateLatestChat(r.wid);
	}
	if (!chat) { return { ok: false, why: 'CHAT_NOT_FOUND' }; }
	return { ok: true, why: '', wid: r.wid, jid: r.jid, chat: chat };
})`

func dispatchScript(toJID, text string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(sendStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(sendStateKey) + `] = v; };
		const resolveChat = ` + resolveChatExpr + `;
		(async () => {
		let stage = 'resolve';
		try {
			const r = await resolveChat(` + strconv.Quote(toJID) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }
			const jid = r.jid;
			const chat = r.chat;

			stage = 'dispatch';			stage = 'dispatch';
			const Send = window.require('` + string(spa.ModuleSendTextMsgChatAction) + `');
			await Send.sendTextMsgToChat(chat, ` + strconv.Quote(text) + `);
			park({ stage, ok: true, why: '', jid: jid });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 120) });
		}
		})();
		return { started: true };
	})())`
}

// verify polls the message collection for an OUTGOING message to this chat,
// stamped after the send began.
//
// Freshness is the discriminator, and it is not decoration: the collection
// replays history, so "an outgoing message to this chat exists" is true for
// every chat that ever had one. Without the timestamp bound this would confirm
// sends that never happened — the exact false positive the delivery test hit.
// resolvedJID, NOT the caller's jid: this build stores messages under the LID
// identity the server returns, so verifying against what the caller typed
// matches nothing. The parameter is named for the distinction because losing it
// costs a false ErrUnverified on a send that succeeded.
// kind narrows what a verification will accept.
//
// It exists because this package now sends more than one thing. Text and media
// land in the same collection, and "an outgoing message appeared" stops being
// evidence for a specific send the moment two kinds can be in flight.
type kind int

const (
	kindAny kind = iota
	kindText
	kindMedia
)

// kindOf maps the page's message type to what this package distinguishes.
//
// Measured on this build: a text message carries type "chat"; media carries
// "image", "document", "video", "audio", "ptt" or "sticker". Anything else is
// neither, and is never accepted as proof of a send.
func kindOf(t string) kind {
	switch t {
	case "chat":
		return kindText
	case "image", "document", "video", "audio", "ptt", "sticker":
		return kindMedia
	}
	return kindAny
}

func verify(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	resolvedJID string, sentAt time.Time, label string, want kind) (Result, error) {

	start := time.Now()
	deadline := start.Add(verifyBudget)
	for probed := false; !probed || time.Now().Before(deadline); probed = true {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, label+"/verify", func(ctx context.Context) error {
			return eval(ctx, verifyScript(resolvedJID), &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("send: verifying: %w", err)
		}
		msgs, err := messagemeta.DecodeWire([]byte(raw))
		if err != nil {
			return Result{}, fmt.Errorf("send: verifying: %w", err)
		}
		for _, m := range msgs {
			if !m.ID.FromMe || !m.ID.Present() {
				continue
			}
			if m.Timestamp.Before(sentAt) {
				continue
			}
			// KIND MATTERS when more than one kind can be in flight. A text
			// send verified by "any outgoing message" would happily accept the
			// media message a concurrent call just produced, and vice versa —
			// the same class of false positive the closed-loop test hit when
			// freshness alone was the discriminator (H35).
			if want != kindAny && kindOf(m.Type) != want {
				continue
			}
			// FOUND — BUT "IT IS IN THE COLLECTION" IS NOT "IT WAS SENT".
			//
			// That was the whole postcondition until H98, and it is exactly
			// what let a poll be reported as sent while sitting at ack 0
			// forever, with the recipient never receiving it. Appearing here
			// and leaving here are different facts and this used to conflate
			// them.
			//
			// The ack is the local evidence that the server has it: 0 pending,
			// 1 server, 2 device, 3 read. Measured on this account (H99):
			// appearance is instant (2ms), ack>=1 lands at ~508ms and ack>=2 at
			// ~1.01s. Half a second is what proving the send costs, against a
			// verification budget of many seconds.
			ack, err := waitForAck(ctx, runner, eval, m.ID.ID, deadline, label)
			if err != nil {
				return Result{}, err
			}
			return Result{ID: m.ID, Timestamp: m.Timestamp, Ack: ack,
				Waited: time.Since(start)}, nil
		}

		time.Sleep(verifyTick)
	}
	return Result{}, fmt.Errorf("%w within %s (recipient not printed)", ErrUnverified, verifyBudget)
}

// verifyScript reuses messagemeta's allow-list, so a verified send cannot carry
// content that a drained event would not — one list, one place to be wrong.
func verifyScript(resolvedJID string) string {
	return `JSON.stringify((() => {
		const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
		if (!coll || typeof coll.getModelsArray !== 'function') { return []; }
		const meta = ` + messagemeta.MetaExpr + `;
		const want = ` + strconv.Quote(resolvedJID) + `;
		const out = [];
		const all = coll.getModelsArray();
		for (let i = all.length - 1; i >= 0 && out.length < 40; i--) {
			let m;
			try { m = meta(all[i]); } catch (e) { continue; }
			if (m.id.remote_jid !== want) { continue; }
			out.push(m);
		}
		return out;
	})())`
}

// waitForAck bounds the wait for a message to reach the server.
//
// IT SHARES THE VERIFICATION DEADLINE rather than adding one of its own. A send
// that has already spent most of its budget finding the message should not then
// be granted a fresh budget to prove it left; the caller asked for one bounded
// operation.
func waitForAck(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	id string, deadline time.Time, label string) (int, error) {

	last := 0
	for {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, label+"/ack", func(ctx context.Context) error {
			return eval(ctx, ackScript(id), &raw)
		}); err != nil {
			return 0, fmt.Errorf("send: reading the ack: %w", err)
		}
		var out struct {
			Found bool `json:"found"`
			Ack   int  `json:"ack"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return 0, fmt.Errorf("send: unexpected ack answer: %w", err)
		}
		if out.Found {
			last = out.Ack
		}
		if last >= 1 {
			return last, nil
		}
		if !time.Now().Before(deadline) {
			return last, fmt.Errorf("%w (ack=%d)", ErrNeverLeft, last)
		}
		time.Sleep(verifyTick)
	}
}

// ackScript reads one message's ack. Synchronous: the clock stays on the Go
// side, as it does for every other wait in this module.
// ackReadMarker is how a test double recognises this script.
//
// IT EXISTS BECAUSE THE OBVIOUS MARKER WAS NOT UNIQUE. The doubles first keyed
// off "m.id && m.id.id ===", which the REPLY dispatch script also contains — so
// the double answered the ack payload to a dispatch and swallowed it, and an
// assertion about the dispatch failed for a reason unrelated to the code under
// test. A script that needs to be recognised should say its own name.
const ackReadMarker = "wa-headless/ack-read"

func ackScript(id string) string {
	return `JSON.stringify((() => {
		const marker = ` + strconv.Quote(ackReadMarker) + `;
		const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
		const all = (coll && typeof coll.getModelsArray === 'function') ? coll.getModelsArray() : [];
		for (const m of all) {
			try {
				if (m.id && m.id.id === ` + strconv.Quote(id) + `) {
					return { found: true, ack: typeof m.ack === 'number' ? m.ack : -1 };
				}
			} catch (e) {}
		}
		return { found: false, ack: 0 };
	})())`
}
