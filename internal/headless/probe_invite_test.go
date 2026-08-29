package headless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeInviteMemo tests the explanation the argument instrument produced for
// H57, which had two symptoms nobody could reconcile: queryGroupInviteCode both
// "throws reading iAmAdmin" and "hangs forever", depending on the run.
//
// The instrument reported that it reads $ProxyState$state.groupInviteCodePromise
// and then returns WITHOUT throwing. That is one function with two exits:
//
//	if (x.groupInviteCodePromise) return x.groupInviteCodePromise;   // memo
//	…iAmAdmin check…                                                  // fresh
//
// which explains both symptoms at once. A recorder makes the memo look present,
// so it returned; a real object with a POISONED memo — left by an earlier
// attempt whose underlying query never settled — returns a promise that never
// resolves, forever.
//
// So this probe asks one question: does clearing the memo let a fresh query run,
// and does that query settle?
func TestProbeInviteMemo(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_INVITE") == "" {
		t.Skip("set WA_PROBE_INVITE=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}

	kick := `(() => {
		window.__inviteProbe = null;
		const park = v => { window.__inviteProbe = JSON.stringify(v); };
		(async () => {
		const out = {};
		try {
			const W = window.require('WAWebWidFactory');
			const C = window.require('WAWebChatCollection').ChatCollection;
			const wid = W.createWid(` + strconv.Quote(gjid) + `);
			const chat = C.get(wid);
			const md = chat && chat.groupMetadata;
			out.hasChat = !!chat; out.hasMd = !!md;
			if (!md) { park(out); return; }

			// WHERE THE MEMO LIVES was never measured; look on both.
			const where = [];
			for (const [name, obj] of [['chat', chat], ['md', md]]) {
				try {
					if (obj.groupInviteCodePromise !== undefined) {
						where.push(name + '=' + (obj.groupInviteCodePromise === null ? 'null' : 'set'));
					}
				} catch (e) {}
			}
			out.memoBefore = where.length ? where : ['neither'];
			out.inviteCodeBefore = (md.inviteCode || chat.inviteCode || null);

			// CLEAR IT ON BOTH, then ask again.
			try { chat.groupInviteCodePromise = null; } catch (e) { out.clearChat = String(e).slice(0, 60); }
			try { md.groupInviteCodePromise = null; } catch (e) { out.clearMd = String(e).slice(0, 60); }

			const A = window.require('WAWebGroupInviteAction');
			// BOUNDED. The whole point is that this call has been observed never
			// to settle; racing it against a timer turns a hang into an answer.
			const settled = await Promise.race([
				A.queryGroupInviteCode(md).then(v => ({ ok: true, v: String(v).slice(0, 40) }),
					e => ({ ok: false, e: String((e && e.message) || e).slice(0, 120) })),
				new Promise(r => setTimeout(() => r({ ok: false, e: 'TIMED_OUT_45s' }), 45000))
			]);
			out.afterClear = settled;
			out.inviteCodeAfter = (md.inviteCode || chat.inviteCode || null) ? 'present' : 'absent';
		} catch (e) {
			out.outer = String((e && e.message) || e).slice(0, 160);
		}
		park(out);
		})();
		return 'started';
	})()`
	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/invite-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 200; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/invite-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__inviteProbe || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe itself never settled")
	}
	t.Logf("%s", raw)
}
