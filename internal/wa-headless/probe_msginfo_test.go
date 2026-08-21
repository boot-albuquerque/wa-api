package waheadless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeMsgInfoShape asks what a sent message's delivery record looks like on
// this build. It reads only, and it prints no identities: participants are
// reported as COUNTS.
func TestProbeMsgInfoShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MSGINFO") == "" {
		t.Skip("set WA_PROBE_MSGINFO=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	var raw string
	script := `JSON.stringify((() => {
		const out = {};
		const MC = window.require('WAWebMsgCollection').MsgCollection;
		const IC = window.require('WAWebMsgInfoCollection').MsgInfoCollection;
		out.infoCount = IC.getModelsArray ? IC.getModelsArray().length : -1;

		const mine = [];
		for (const m of MC.getModelsArray()) {
			try { if (m.id && m.id.fromMe) { mine.push(m); } } catch (e) {}
		}
		mine.sort((a, b) => (b.t || 0) - (a.t || 0));
		out.fromMeCount = mine.length;

		const rows = [];
		for (const m of mine.slice(0, 5)) {
			const row = { t: m.t || 0, ack: m.ack, type: m.type || null };
			try {
				const info = IC.get ? IC.get(m.id) : null;
				row.hasInfo = !!info;
				if (info) {
					row.infoKeys = Object.keys(info).filter(k => !k.startsWith('__')).slice(0, 20);
					for (const f of ['delivery', 'read', 'played', 'deliveryRemaining', 'readRemaining']) {
						const v = info[f];
						if (v === undefined) { continue; }
						row[f] = (v && v.length !== undefined) ? v.length
							: (typeof v === 'number' ? v : typeof v);
					}
				}
			} catch (e) { row.hasInfo = 'THREW:' + String(e).slice(0, 60); }
			rows.push(row);
		}
		out.newest = rows;
		try {
			const G = window.require('WAWebMsgInfoGetters');
			out.getterKeys = Object.keys(G).slice(0, 20);
		} catch (e) { out.getterKeys = 'THREW'; }
		return out;
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/msginfo", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("%s", raw)
}
