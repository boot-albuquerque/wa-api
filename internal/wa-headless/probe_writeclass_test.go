package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
	"wa-api/internal/wa-headless/spa"
)

// TestProbeWriteConfirmationClass measures whether a write is visible to the
// session that made it, for three cases whose answers are ALREADY KNOWN.
//
// The controls come first for the same reason they did for the argument
// instrument (H73): a classifier that cannot reproduce three measured answers
// has not earned the unmeasured ones.
//
//	star            expected IMMEDIATE     (H61, 696ms)
//	group policy    expected not-here      (H79, cross-session)
//	message pin     expected not-here      (H81, nothing happens anywhere)
//
// The last two look alike from inside one session, and that is the point:
// distinguishing them needs a second session, which is the step every failed
// capability skipped.
//
// THE STARRING CONTROL TOGGLES rather than restores — it stars if unstarred and
// unstars if starred. That is deliberate: it always produces a change to observe,
// and starring is local to this account, so the only cost is a star that may be
// on or off afterwards on one of our own test messages.
//
// EVERY OTHER CASE IS A NO-OP. The policy
// is written with the value it already has; the pin is the one write that has
// been measured to do nothing, and a fresh session confirmed nothing is pinned.
func TestProbeWriteConfirmationClass(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_WRITECLASS") == "" {
		t.Skip("set WA_PROBE_WRITECLASS=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}

	// Each case is (name, expectation, writer, reader) as page functions.
	cases := []struct{ name, expect, write, read string }{
		{
			"star (control: known IMMEDIATE)", "IMMEDIATE",
			`async () => {
				const MC = window.require('WAWebMsgCollection').MsgCollection;
				const m = MC.getModelsArray().find(x => x.id && x.id.fromMe && x.type === 'chat');
				if (!m) { throw new Error('no own text message'); }
				window.__wcMsg = m;
				const chat = window.require('WAWebChatCollection').ChatCollection.get(m.id.remote);
				const Cmd = window.require('WAWebCmd').Cmd;
				await (m.star ? Cmd.sendUnstarMsgs(chat, [m], true) : Cmd.sendStarMsgs(chat, [m], true));
			}`,
			`() => {
				const MC = window.require('WAWebMsgCollection').MsgCollection;
				const m = window.__wcMsg || MC.getModelsArray().find(x => x.id && x.id.fromMe && x.type === 'chat');
				return m ? !!m.star : null;
			}`,
		},
		{
			// A REAL FLIP, AND THE CONTROL THAT WAS WRONG BEFORE. This case used
			// to write the value the group already had, to avoid changing
			// anything — and a no-op CANNOT move a reader, so it reported
			// "not-here" for the trivial reason that nothing changed. That
			// wrong answer then confirmed a wrong prior: policies were filed as
			// CROSS_SESSION when they are visible here in about one second.
			//
			// A control that cannot fail is not a control. This one flips and
			// flips back, which is a real change and is restored.
			"group policy (control: known IMMEDIATE, ~1s)", "IMMEDIATE",
			`async () => {
				const W = window.require('WAWebWidFactory');
				const C = window.require('WAWebChatCollection').ChatCollection;
				const chat = C.get(W.createWid(` + strconv.Quote(gjid) + `));
				const md = chat.groupMetadata;
				const A = window.require('WAWebSetPropertyGroupAction');
				window.__wcPolicyWas = !!md.announce;
				await A.setGroupProperty(chat, 'announcement', md.announce ? 0 : 1);
			}`,
			`() => {
				const W = window.require('WAWebWidFactory');
				const C = window.require('WAWebChatCollection').ChatCollection;
				const chat = C.get(W.createWid(` + strconv.Quote(gjid) + `));
				return chat && chat.groupMetadata ? !!chat.groupMetadata.announce : null;
			}`,
		},
		{
			// THE PENDING HYPOTHESIS FROM H81, tested with the classifier
			// instead of another whole capability: the bridge plus the local
			// mirror, which is the pair H72 measured for labels. If the mirror
			// is what was missing, this turns NOTHING into IMMEDIATE.
			"pin WITH the local mirror (H81 hypothesis)", "IMMEDIATE",
			`async () => {
				const MC = window.require('WAWebMsgCollection').MsgCollection;
				const m = MC.getModelsArray().find(x => x.id && x.id.fromMe && x.type === 'chat');
				if (!m) { throw new Error('no own text message'); }
				const K = window.require('WAWebPinMsgConstants');
				const A = window.require('WAWebSendPinMessageAction');
				const P = window.require('WAWebPinMessageAction');
				const secs = Number(K.getPinExpiryDuration(K.DEFAULT_PIN_EXPIRY_DURATION_OPTION)) || 0;
				await A.sendPinInChatMsg(m, K.PIN_STATE.PIN, secs);
				// THE MIRROR. updatePinCollection measured as wanting something
				// iterable, so it is handed the crafted message in a list.
				try {
					const crafted = await P.craftPinMessage(m, K.PIN_STATE.PIN, secs);
					await P.updatePinCollection([crafted]);
				} catch (e) { window.__pinMirrorErr = String((e && e.message) || e).slice(0, 160); }
			}`,
			`() => {
				const P = window.require('WAWebPinInChatCollection').PinInChatCollection;
				return P && P.getModelsArray ? P.getModelsArray().length : null;
			}`,
		},
		{
			"message pin (control: known NOTHING)", "not-here",
			`async () => {
				const MC = window.require('WAWebMsgCollection').MsgCollection;
				const m = MC.getModelsArray().find(x => x.id && x.id.fromMe && x.type === 'chat');
				if (!m) { throw new Error('no own text message'); }
				const K = window.require('WAWebPinMsgConstants');
				const A = window.require('WAWebSendPinMessageAction');
				await A.sendPinInChatMsg(m, K.PIN_STATE.UNPIN, 0);
			}`,
			`() => {
				const P = window.require('WAWebPinInChatCollection').PinInChatCollection;
				return P && P.getModelsArray ? P.getModelsArray().length : null;
			}`,
		},
	}

	// The mirror hypothesis records its own failure on the page; reading it
	// afterwards says whether the mirror threw or ran and did nothing, which are
	// different answers.
	defer func() {
		var e string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/wc-mirror", func(x context.Context) error {
			return sess.Tab().Evaluate(x, `window.__pinMirrorErr || "no error"`, &e)
		}); err == nil {
			t.Logf("MIRROR: %s", e)
		}
	}()

	// The policy control changes a real setting; it is put back whatever the
	// rest of the run does.
	defer func() {
		var was string
		if err := runner.Do(context.Background(), engine.OpStateProbe, "probe/wc-restore", func(x context.Context) error {
			return sess.Tab().Evaluate(x, `(() => {
				try {
					if (window.__wcPolicyWas === undefined) { return 'nothing to restore'; }
					const W = window.require('WAWebWidFactory');
					const C = window.require('WAWebChatCollection').ChatCollection;
					const chat = C.get(W.createWid(`+strconv.Quote(gjid)+`));
					// PARKED, NOT AWAITED HERE: Evaluate does not await, so the
					// promise this returns would be abandoned. An earlier
					// version called it and returned immediately, and the
					// restore did not take — the lab group carried a flipped
					// policy until it was noticed.
					window.__wcRestore = 'pending';
					window.require('WAWebSetPropertyGroupAction')
						.setGroupProperty(chat, 'announcement', window.__wcPolicyWas ? 1 : 0)
						.then(() => { window.__wcRestore = 'done'; },
						      e => { window.__wcRestore = 'FAILED: ' + String(e).slice(0, 100); });
					return 'asked to restore to ' + String(window.__wcPolicyWas);
				} catch (e) { return 'RESTORE FAILED: ' + String(e).slice(0, 120); }
			})()`, &was)
		}); err != nil {
			t.Errorf("restoring the policy: %v", err)
			return
		}
		t.Logf("policy control: %s", was)
		// AND WAITED FOR, from Go. A restore that is only asked for is a
		// restore that may not happen.
		for i := 0; i < 40; i++ {
			var st string
			if err := runner.Do(context.Background(), engine.OpStateProbe, "probe/wc-restore-poll", func(x context.Context) error {
				return sess.Tab().Evaluate(x, `window.__wcRestore || "unknown"`, &st)
			}); err != nil {
				break
			}
			if st != "pending" {
				t.Logf("policy control restore: %s", st)
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Error("the policy control restore never settled; the lab group may carry a flipped policy")
	}()

	for _, c := range cases {
		kick := `(() => { window.__wc = null;
			return (` + spa.ClassifyWriteExpr + `)("__wc", ` + c.write + `, ` + c.read + `); })()`
		var started string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/wc-kick", func(x context.Context) error {
			return sess.Tab().Evaluate(x, kick, &started)
		}); err != nil {
			t.Errorf("%s: kick: %v", c.name, err)
			continue
		}
		// POLLED FROM GO, and long enough to outlast the 696ms the starring
		// control needs. The page never decides how long to wait.
		read := `(` + spa.ClassifyReadExpr + `)("__wc")`
		var raw string
		settled := false
		for i := 0; i < 24; i++ {
			if err := runner.Do(ctx, engine.OpStateProbe, "probe/wc-read", func(x context.Context) error {
				return sess.Tab().Evaluate(x, read, &raw)
			}); err != nil {
				t.Errorf("%s: read: %v", c.name, err)
				break
			}
			// PENDING means the write is still running and SETTLING means it
			// finished and the reader has not moved yet. Both are "keep
			// waiting" — an earlier version broke on pending and reported two
			// controls as refusals, which is the same class of mistake as
			// reading straight after the await.
			if raw != "" && !contains(raw, `"stage":"settling"`) && !contains(raw, `"stage":"pending"`) {
				settled = true
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		_ = settled
		if raw == "" {
			t.Errorf("%s: never answered", c.name)
			continue
		}
		var out struct {
			OK            bool   `json:"ok"`
			Why           string `json:"why"`
			Before, After string `json:"-"`
			Moved         bool   `json:"moved"`
			BeforeS       string `json:"before"`
			AfterS        string `json:"after"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Errorf("%s: %v (%.160s)", c.name, err, raw)
			continue
		}
		got := "not-here"
		if !out.OK {
			got = "REFUSED"
		} else if out.Moved {
			got = "IMMEDIATE"
		}
		verdict := "AS EXPECTED"
		if got != c.expect {
			verdict = "*** DISAGREES WITH THE RECORDED ANSWER ***"
		}
		t.Logf("%-44s before=%-6s after=%-6s -> %-10s (expected %s) %s\n    why=%s\n    raw=%.220s",
			c.name, out.BeforeS, out.AfterS, got, c.expect, verdict, out.Why, raw)
	}
}
