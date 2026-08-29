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

// TestProbeStatusCollection measures the family the ledger calls "broadcast".
//
// THE NAME IN THE LEDGER IS WRONG AND THAT IS THE FIRST FINDING. Our note says
// "família de listas de transmissão" — broadcast lists. Reading the reference
// says otherwise: Client.getBroadcasts is window.WWebJS.getAllStatuses, which is
// WAWebCollections.Status.getModelsArray, and the Broadcast structure carries
// msgs, totalCount and unreadCount keyed by a CONTACT id.
//
// That is WhatsApp Status — stories — not a broadcast list. Three ledger rows
// were about to be implemented against the wrong idea.
//
// READ ONLY, and identity-free: counts and field names, never a contact, never
// a caption, never a media url.
func TestProbeStatusCollection(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_STATUS") == "" {
		t.Skip("set WA_PROBE_STATUS=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	const kick = `(() => {
		window.__st = null;
		const park = v => { window.__st = JSON.stringify(v); };
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 130);
		(async () => {
		const out = { modules: {}, collection: {} };
		for (const name of ['WAWebCollections', 'WAWebStatusCollection',
			'WAWebStatusModel', 'WAWebSendStatusMsgAction', 'WAWebStatusGetters']) {
			try {
				const m = window.require(name);
				out.modules[name] = m ? Object.keys(m).slice(0, 25) : 'falsy';
			} catch (e) { out.modules[name] = 'ABSENT'; }
		}
		try {
			const C = window.require('WAWebCollections');
			const S = C && C.Status;
			out.collection.present = !!S;
			if (S) {
				out.collection.hasModelsArray = typeof S.getModelsArray === 'function';
				const all = typeof S.getModelsArray === 'function' ? S.getModelsArray() : [];
				out.collection.count = all.length;
				// FIELD NAMES ONLY. A status carries captions and media urls.
				if (all.length) {
					out.collection.fields = Object.keys(all[0]).filter(k => k.indexOf('__x_') === 0);
					const first = all[0];
					out.collection.sample = {
						hasMsgs: !!(first.msgs),
						msgCount: first.msgs && typeof first.msgs.getModelsArray === 'function'
							? first.msgs.getModelsArray().length
							: (first.msgs && first.msgs.length) || 0,
						totalCount: typeof first.totalCount === 'number' ? first.totalCount : 'absent',
						unreadCount: typeof first.unreadCount === 'number' ? first.unreadCount : 'absent',
						hasT: typeof first.t === 'number',
					};
				}
			}
		} catch (e) { out.collection.threw = safe(e); }
		park(out);
		})();
		return 'kicked';
	})()`

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/status-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 60; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/status-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__st || ""`, &raw)
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
