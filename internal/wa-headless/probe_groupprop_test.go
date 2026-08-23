package waheadless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeGroupPropertyNames enumerates the property names setGroupProperty
// accepts, using the app's OWN rejection as the oracle:
//
//	default: return Promise.reject(err("invalid group property " + a))
//
// An unknown name is refused in the switch BEFORE anything is sent, so asking is
// free. A known name proceeds — so each candidate is passed the group's CURRENT
// value, which makes a valid call a semantic no-op.
//
// That last part is the whole reason this is safe to run: without it, probing
// the names would silently change the lab group's policies one candidate at a
// time.
func TestProbeGroupPropertyNames(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_GROUPPROP") == "" {
		t.Skip("set WA_PROBE_GROUPPROP=1")
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
		window.__gp = null;
		const park = v => { window.__gp = JSON.stringify(v); };
		(async () => {
		const out = { tried: [] };
		try {
			const W = window.require('WAWebWidFactory');
			const C = window.require('WAWebChatCollection').ChatCollection;
			const chat = C.get(W.createWid(` + strconv.Quote(gjid) + `));
			const md = chat && chat.groupMetadata;
			if (!md) { out.why = 'NO_METADATA'; park(out); return; }
			out.current = { restrict: !!md.restrict, announce: !!md.announce };
			out.canSet = (typeof md.canSetGroupProperty === 'function') ? md.canSetGroupProperty() : 'absent';

			const A = window.require('WAWebSetPropertyGroupAction');
			const candidates = ['announcement', 'restrict', 'locked', 'announce',
				'membership_approval_mode', 'allow_admin_reports', 'group_history',
				'no_frequently_forwarded', 'allow_non_admin_sub_group_creation',
				'ephemeral', 'description', 'subject'];
			for (const name of candidates) {
				// THE CURRENT VALUE, so a valid name is a no-op rather than a
				// change. restrict/announce are the two we can read; the rest
				// get 0, which is "off" and is what a fresh lab group has.
				let cur = 0;
				if (name === 'restrict' || name === 'locked') { cur = md.restrict ? 1 : 0; }
				if (name === 'announcement' || name === 'announce') { cur = md.announce ? 1 : 0; }
				let verdict = 'accepted';
				try {
					await A.setGroupProperty(chat, name, cur);
				} catch (e) {
					const m = String((e && e.message) || e);
					verdict = m.indexOf('invalid group property') >= 0 ? 'INVALID' : ('threw: ' + m.slice(0, 90));
				}
				out.tried.push(name + ' -> ' + verdict);
			}
		} catch (e) {
			out.outer = String((e && e.message) || e).slice(0, 160);
		}
		park(out);
		})();
		return 'started';
	})()`
	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/gp-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 120; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/gp-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__gp || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe never settled")
	}
	t.Logf("%s", raw)
}
