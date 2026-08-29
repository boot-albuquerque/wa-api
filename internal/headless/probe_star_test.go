package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeStarShape settles by EXPERIMENT what reading could not.
//
// sendStarMsgs is an async wrapper, so its toString() shows only
// `function u(e,t,n){return d(t,n)}` — enough to know the first argument is
// discarded and not enough to know what the other two are. ARMADILHAS.md
// records that limit; this is the other half of the answer.
//
// AN EXPERIMENT IS A LEGITIMATE MEASUREMENT WHEN IT IS CHEAP AND REVERSIBLE,
// and starring is the safest outward effect in this whole surface: the flag is
// LOCAL to the account, the peer never sees it, and unstarring puts it back.
// Each candidate shape is tried one at a time and judged by whether the
// StarredMsgCollection moved — the same one-variable discipline H58 needed and
// did not get.
//
// It prints counts and shape names. No message content.
func TestProbeStarShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_STAR") == "" {
		t.Skip("set WA_PROBE_STAR=1; this stars and unstars one of this account's own messages")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	kick := `JSON.stringify((() => {
		window.__waStarProbe = { stage: 'pending' };
		const park = (v) => { window.__waStarProbe = v; };
		(async () => {
		const log = [];
		try {
			const MC = window.require('WAWebMsgCollection').MsgCollection;
			const SC = window.require('WAWebStarredMsgCollection');
			const starred = SC.StarredMsgCollection || SC.AllStarredMsgsCollection;
			const count = () => { try { return starred.getModelsArray().length; } catch (e) { return -1; } };

			let msg = null;
			for (const m of MC.getModelsArray()) {
				if (m && m.id && m.id.fromMe && m.type === 'chat' && !m.star) {
					if (!msg || (m.t || 0) > (msg.t || 0)) { msg = m; }
				}
			}
			if (!msg) { park({ stage: 'find', ok: false, why: 'NO_UNSTARRED_OWN_MESSAGE', log }); return; }
			const chat = window.require('WAWebChatCollection').ChatCollection.get(msg.id.remote);
			if (!chat) { park({ stage: 'find', ok: false, why: 'NO_CHAT', log }); return; }

			const base = count();
			log.push({ step: 'baseline', starred: base, msgStar: !!msg.star });

			const Bridge = window.require('WAWebChatSendStarMsgsBridge');
			const Cmd = window.require('WAWebCmd').Cmd;

			const candidates = [
				{ name: 'Cmd.sendStarMsgs(chat, [msg], true)',
				  run: () => Cmd.sendStarMsgs(chat, [msg], true) },
				{ name: 'Bridge.sendStarMsgs(chat.id, [msg], true)',
				  run: () => Bridge.sendStarMsgs(chat.id, [msg], true) },
				{ name: 'Bridge.sendStarMsgs(chat.id, [msg.id], true)',
				  run: () => Bridge.sendStarMsgs(chat.id, [msg.id], true) }
			];

			let winner = null;
			for (const c of candidates) {
				let threw = null;
				try { await c.run(); } catch (e) { threw = String((e && e.message) || e).slice(0, 120); }
				// The page needs a turn to reflect it.
				await new Promise(r => setTimeout(r, 800));
				const now = count();
				const flag = !!msg.star;
				log.push({ step: c.name, threw: threw, starred: now, msgStar: flag });
				if (flag || now > base) { winner = c.name; break; }
			}

			// RESTORE, whatever happened.
			let restoreNote = 'not needed';
			if (!!msg.star) {
				try {
					await Cmd.sendUnstarMsgs(chat, [msg], true);
					await new Promise(r => setTimeout(r, 800));
					restoreNote = 'Cmd.sendUnstarMsgs -> msgStar=' + (!!msg.star);
				} catch (e) { restoreNote = 'THREW:' + String((e && e.message) || e).slice(0, 120); }
			}
			park({ stage: 'done', ok: true, winner: winner, restore: restoreNote,
				finalStarred: count(), log: log });
		} catch (e) {
			park({ stage: 'outer', ok: false, why: String((e && e.message) || e).slice(0, 200), log });
		}
		})();
		return { started: true };
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/star-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &raw)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	read := `JSON.stringify(window.__waStarProbe || {stage:'missing'})`
	for i := 0; i < 40; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/star-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, read, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if !contains(raw, `"stage":"pending"`) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("%s", raw)
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
