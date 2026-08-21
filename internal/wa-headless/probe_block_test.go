package waheadless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeBlockShape is a MEASUREMENT, and it exists because H58 cost a live
// failure that a read would have prevented. The bundle grep already showed that
// blockContact THROWS for a pn contact without a chat; what it could not show is
// the argument list and the entry-point vocabulary. A function's own toString()
// can, and it is exact where a regex around a minified literal is not.
//
// It prints no identity: the blocklist is reported as a COUNT.
func TestProbeBlockShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_BLOCK") == "" {
		t.Skip("set WA_PROBE_BLOCK=1")
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
		const src = (mod, fn) => {
			try {
				const m = window.require(mod);
				const f = m && m[fn];
				return typeof f === 'function' ? String(f).slice(0, 900) : ('NOT_A_FUNCTION:' + typeof f);
			} catch (e) { return 'THREW:' + String((e && e.message) || e).slice(0, 120); }
		};
		for (const m of ['WAWebSetAboutJob', 'WAWebSetTextStatusJob']) {
			try {
				const mod = window.require(m);
				const o3 = mod && (mod.setAbout || mod.setTextStatus);
				out[m + '_shape'] = {
					type: typeof o3,
					isArray: Array.isArray(o3),
					length: o3 && o3.length,
					zeroType: o3 && typeof o3[0],
					zeroSrc: (o3 && typeof o3[0] === 'function') ? String(o3[0]).slice(0, 400) : null,
					ctor: o3 && o3.constructor && o3.constructor.name
				};
			} catch (e) { out[m + '_shape'] = 'THREW:' + String(e).slice(0, 80); }
		}
		for (const m of []) {
			try {
				const mod = window.require(m);
				const o2 = mod && (mod.setAbout || mod.setTextStatus);
				out[m] = o2 ? Object.keys(o2).concat(
					Object.getPrototypeOf(o2) ? Object.keys(Object.getPrototypeOf(o2)) : []) : 'NULL';
				out[m + '_run'] = (o2 && typeof o2.run === 'function') ? String(o2.run).slice(0, 400) : null;
				out[m + '_modKeys'] = Object.keys(mod);
			} catch (e) { out[m] = 'THREW:' + String(e).slice(0, 90); }
		}
		try {
			const C = window.require('WAWebConnModel').Conn;
			out.canSetMyPushname = !!(C.canSetMyPushname && C.canSetMyPushname());
			out.pushnameLen = (C.pushname || '').length;
		} catch (e) { out.canSetMyPushname = 'THREW'; }
		out.blockContact = src('WAWebBlockContactAction', 'blockContact');
		out.unblockContact = src('WAWebBlockContactAction', 'unblockContact');
		out.blockUnblockUser = src('WAWebBlockUserJob', 'blockUnblockUser');
		try {
			const C = window.require('WAWebBlockContants');
			out.entryPoints = C && C.BlockEntryPoint ? Object.keys(C.BlockEntryPoint) : null;
			out.entryPointValues = C && C.BlockEntryPoint ? Object.values(C.BlockEntryPoint).slice(0, 30) : null;
		} catch (e) { out.entryPoints = 'THREW:' + String(e).slice(0, 80); }
		try {
			const B = window.require('WAWebBlocklistCollection').BlocklistCollection;
			const arr = B.getModelsArray();
			out.blocklistCount = arr.length;
			out.blocklistKeys = arr.length ? Object.keys(arr[0]).slice(0, 20) : [];
		} catch (e) { out.blocklistCount = 'THREW:' + String(e).slice(0, 80); }
		return out;
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/block", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("%s", raw)
}
