package send

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

// Replying to a message.
//
// THIS ONE RESTS ON AN INFERENCE, AND SAYS SO. Everything else in this package
// had its call shape read from the app's own code; here the reading went only
// this far:
//
//	createTextMsgData(chat, text, options)  builds the message data
//	quotedMsg                               is the field a reply carries
//	sendTextMsgToChat(chat, text, options)  forwards options to createTextMsgData
//
// What was NOT found in the bundles is the app calling sendTextMsgToChat with a
// quote — the reply UI goes through its own composer path. So passing
// {quotedMsg} through the options is an inference from two measured facts, not
// a measured fact itself.
//
// ONE MISREADING ALREADY COST A RUN: createQuotedMsgObj looked like the way to
// build the quote, and it is not — its source requires quotedStanzaID and
// returns null without one, because it converts a message that ALREADY IS a
// reply into its quoted object, for rendering. The value quotedMsg wants is the
// message being replied to.
//
// The live proof is therefore load-bearing rather than confirmatory: it checks
// that the sent message actually carries a quote, and a failure there means the
// inference was wrong and the composer path has to be read instead.

var (
	// ErrNoQuoted is a message id that is not in the loaded collection. Replying
	// to something the page has not loaded is not possible, and pretending
	// otherwise would send an ordinary message that silently is not a reply.
	ErrNoQuoted = fmt.Errorf("send: the quoted message is not in the loaded collection")
	// ErrNotAReply is the postcondition: the message went out and carries no
	// quote. It is the failure a caller cannot see — the text arrives either
	// way, and only the missing quote distinguishes them.
	ErrNotAReply = fmt.Errorf("send: the message was sent but carries no quote")
)

const replyStateKey = "__waHeadlessReplyResult"

// Reply sends text as a reply to an existing message.
//
// It verifies that what went out is actually a reply, because a reply that
// loses its quote is indistinguishable from an ordinary message at the sending
// end and completely different at the receiving one.
func Reply(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	toJID, quotedMsgID, text, label string) (Result, error) {

	if strings.TrimSpace(toJID) == "" {
		return Result{}, ErrNoChat
	}
	if strings.TrimSpace(quotedMsgID) == "" {
		return Result{}, ErrNoQuoted
	}
	sentAt := time.Now().Add(-clockSkewAllowance)

	var kicked string
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return eval(ctx, replyScript(toJID, quotedMsgID, text), &kicked)
	}); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrDispatch, err)
	}

	var out struct {
		Stage string `json:"stage"`
		OK    bool   `json:"ok"`
		Why   string `json:"why"`
		JID   string `json:"jid"`
	}
	deadline := time.Now().Add(verifyBudget)
	for {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, label+"/result", func(ctx context.Context) error {
			return eval(ctx, replyResultScript, &raw)
		}); err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrDispatch, err)
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return Result{}, fmt.Errorf("send: unexpected reply answer: %w", err)
		}
		if out.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return Result{}, fmt.Errorf("%w: the page never settled the reply within %s",
				ErrDispatch, verifyBudget)
		}
		time.Sleep(verifyTick)
	}
	switch {
	case out.Stage == "quote" && !out.OK:
		return Result{}, fmt.Errorf("%w (%s)", ErrNoQuoted, out.Why)
	case out.Stage == "resolve" && !out.OK:
		return Result{}, fmt.Errorf("%w (%s)", ErrNoChat, out.Why)
	case !out.OK:
		return Result{}, fmt.Errorf("%w at %s (%s)", ErrDispatch, out.Stage, out.Why)
	}
	if out.JID == "" {
		return Result{}, fmt.Errorf("%w: the page reported success without a resolved identity", ErrDispatch)
	}

	res, err := verify(ctx, runner, eval, out.JID, sentAt, label, kindText)
	if err != nil {
		return Result{}, err
	}
	// THE POSTCONDITION THAT MAKES THIS A REPLY. The text arrives either way;
	// only the quote distinguishes a reply from an ordinary message, and the
	// sender cannot see the difference without asking.
	quoted, err := replyCarriesQuote(ctx, runner, eval, res.ID.ID, label)
	if err != nil {
		return Result{}, err
	}
	if !quoted {
		return Result{}, ErrNotAReply
	}
	return res, nil
}

// replyCarriesQuote asks whether the message that went out is a reply.
func replyCarriesQuote(ctx context.Context, runner *engine.Runner, eval spa.Evaluator,
	msgID, label string) (bool, error) {

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, label+"/quoted", func(ctx context.Context) error {
		return eval(ctx, quotedCheckScript(msgID), &raw)
	}); err != nil {
		return false, fmt.Errorf("%w: %v", ErrDispatch, err)
	}
	var got struct {
		Found  bool `json:"found"`
		Quoted bool `json:"quoted"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return false, fmt.Errorf("send: unexpected quote answer: %w", err)
	}
	if !got.Found {
		return false, ErrUnverified
	}
	return got.Quoted, nil
}

func quotedCheckScript(msgID string) string {
	return `JSON.stringify((() => {
		const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
		const want = ` + strconv.Quote(msgID) + `;
		for (const m of coll.getModelsArray()) {
			try {
				if (!m.id || m.id.id !== want) { continue; }
				// Two names for the same fact, because the model carries the
				// built object and the flag separately and either one being
				// present is the message being a reply.
				return { found: true, quoted: !!(m.quotedMsg || m.quotedStanzaID || m.fromQuotedMsg) };
			} catch (e) {}
		}
		return { found: false, quoted: false };
	})())`
}

func replyScript(toJID, quotedMsgID, text string) string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(replyStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(replyStateKey) + `] = v; };
		const resolveChat = ` + resolveChatExpr + `;
		(async () => {
		let stage = 'quote';
		try {
			const coll = window.require('` + string(spa.ModuleMsgCollection) + `').MsgCollection;
			const wantID = ` + strconv.Quote(quotedMsgID) + `;
			let target = null;
			for (const m of coll.getModelsArray()) {
				try { if (m.id && m.id.id === wantID) { target = m; break; } } catch (e) {}
			}
			if (!target) { park({ stage, ok: false, why: 'NOT_LOADED' }); return; }

			// NOT createQuotedMsgObj. Its source requires e.quotedStanzaID and
			// returns null without one — it converts a message that ALREADY IS
			// a reply into its quoted object, for rendering. Passing the target
			// there returned null, which is how the misreading was caught.
			//
			// What a reply carries is quotedMsg, and the value is the message
			// being replied to.
			const quoted = target;

			stage = 'resolve';
			const r = await resolveChat(` + strconv.Quote(toJID) + `);
			if (!r.ok) { park({ stage, ok: false, why: r.why }); return; }

			stage = 'dispatch';
			const Send = window.require('` + string(spa.ModuleSendTextMsgChatAction) + `');
			// The third argument is the options object createTextMsgData
			// receives, and quotedMsg is the field a reply carries. This is the
			// one inference in this package rather than a read fact, which is
			// why the caller verifies the quote afterwards.
			await Send.sendTextMsgToChat(r.chat, ` + strconv.Quote(text) + `, { quotedMsg: quoted });
			park({ stage: 'done', ok: true, why: '', jid: r.jid });
		} catch (e) {
			park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
		}
		})();
		return { started: true };
	})())`
}

const replyResultScript = `JSON.stringify((() => {
	const s = window[` + `"` + replyStateKey + `"` + `];
	if (!s) { return { stage: 'dispatch', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`
