package waheadless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/call"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeCallInFlight is the measurement H93 could not make.
//
// It stopped at a zero it could not interpret: conta-A read no calls after
// dialling, and the reader had never been observed counting one. Two different
// things produce that zero — a dial that did nothing, and a reader that counts
// nothing — and separating them needs the collection watched WHILE a call is
// supposed to exist.
//
// So this places a call and then dumps every container on the call collection,
// by name and size, once a second. Whatever moves is the reader; if nothing
// moves anywhere, the dial is what is broken, and that is a conclusion the
// earlier run was not entitled to.
//
// It cancels the outgoing call on every exit path.
func TestProbeCallInFlight(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_INCOMING_CALL") == "" {
		t.Skip("set WA_REAL_INCOMING_CALL=1 — THIS PLACES A REAL CALL and may ring a phone")
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
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	cm := call.New(runner, sess.Tab().Evaluate)

	// EVERY CONTAINER, BY NAME AND SIZE, and the REASON when a read throws —
	// "threw" alone was the previous version's answer and it named nothing.
	const dump = `JSON.stringify((() => {
		const out = {};
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 90);
		try {
			const C = window.require('WAWebCallCollection');
			for (const k of Object.keys(C)) {
				try {
					const v = C[k];
					if (v instanceof Map) { out[k] = 'Map:' + v.size; continue; }
					if (Array.isArray(v)) { out[k] = 'arr:' + v.length; continue; }
					if (v && typeof v.getModelsArray === 'function') { out[k] = 'coll:' + v.getModelsArray().length; continue; }
					if (v === null || v === undefined) { out[k] = String(v); continue; }
					out[k] = typeof v;
				} catch (e) { out[k] = 'threw(' + safe(e) + ')'; }
			}
			const holder = C.CallCollection || C.default || C;
			out['#models'] = holder && typeof holder.getModelsArray === 'function'
				? holder.getModelsArray().length : -1;
		} catch (e) { out['#fatal'] = safe(e); }
		return out;
	})())`

	read := func(when string) string {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/inflight", func(c context.Context) error {
			return sess.Tab().Evaluate(c, dump, &raw)
		}); err != nil {
			t.Fatalf("%s: %v", when, err)
		}
		return raw
	}

	before := read("baseline")
	t.Logf("BEFORE: %s", before)

	// The VOIP stack, then the dial. EnsureReady is separate from Place on
	// purpose; here both run so the failure cannot be blamed on the order.
	if err := cm.EnsureReady(ctx, "inflight/ready"); err != nil {
		t.Fatalf("EnsureReady: %v", err)
	}
	defer func() {
		if err := cm.Cancel(context.Background(), "inflight/cancel"); err != nil {
			t.Logf("cancelling: %v", err)
		}
	}()
	// THE APP NAVIGATES TO THE CALLS TAB BEFORE IT DIALS, and this run is what
	// tests whether that matters.
	//
	// Its own call sites read:
	//   Cmd.setActiveNavBarItem(NavBarItems.Calls)
	//   navigateToVoipCallsTab({})
	//   queryWidExists(...).then(e => startWAWebVoipCall(e.wid, ...))
	//
	// H78 established that Cmd is an EVENT BUS: a verb only acts if a listener
	// is bound, and the listeners live in UI pieces a headless session never
	// mounts. If the VOIP handlers are bound by the calls tab, a driver that
	// never opens it is dialling into nothing — which is exactly what the
	// collection just showed.
	//
	// Driving UI is authorised only as a MEASUREMENT INSTRUMENT, never in
	// production, and that is all this is.
	if os.Getenv("WA_PROBE_CALLS_TAB") != "" {
		const openTab = `JSON.stringify((() => {
			const out = {};
			try {
				const Cmd = window.require('WAWebCmd').Cmd;
				const NB = window.require('WAWebNavBarTypes').NavBarItems;
				Cmd.setActiveNavBarItem(NB.Calls);
				out.navBar = 'ok';
			} catch (e) { out.navBar = 'threw: ' + String((e && e.message) || e).slice(0, 90); }
			try {
				window.require('WAWebVoipCallsTabNavigateTo').navigateToVoipCallsTab({});
				out.tab = 'ok';
			} catch (e) { out.tab = 'threw: ' + String((e && e.message) || e).slice(0, 90); }
			return out;
		})())`
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/callstab", func(c context.Context) error {
			return sess.Tab().Evaluate(c, openTab, &raw)
		}); err != nil {
			t.Fatalf("opening the calls tab: %v", err)
		}
		t.Logf("calls tab: %s", raw)
		time.Sleep(3 * time.Second)
		before = read("after opening the calls tab")
		t.Logf("BEFORE (post-tab): %s", before)
	}

	// WHAT DOES THE DIAL ACTUALLY RETURN? The capability awaits it and discards
	// the value, which is right for a capability and useless for a diagnosis.
	// Four hypotheses are dead; the function's own answer has never been read.
	const dial = `(() => {
		window.__dial = null;
		const park = v => { window.__dial = JSON.stringify(v); };
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 160);
		(async () => {
			try {
				const W = window.require('WAWebWidFactory');
				const ex = await window.require('WAWebQueryExistsJob')
					.queryWidExists(W.createWid(PEER));
				if (!ex || !ex.wid) { park({ ok: false, why: 'NOT_ON_WHATSAPP' }); return; }
				await window.require('WAWebEnsureVoipInited').ensureVoipInitialized();
				const r = await window.require('WAWebVoipStartCall').startWAWebVoipCall(ex.wid, false);
				park({
					ok: true,
					kind: typeof r,
					isNull: r === null,
					keys: (r && typeof r === 'object') ? Object.keys(r).slice(0, 25) : [],
					str: (typeof r === 'string' || typeof r === 'number' || typeof r === 'boolean')
						? String(r) : '',
				});
			} catch (e) {
				park({ ok: false, why: safe(e),
					name: (e && e.constructor && e.constructor.name) || typeof e,
					keys: (e && typeof e === 'object') ? Object.keys(e).slice(0, 25) : [] });
			}
		})();
		return 'kicked';
	})()`
	var kicked string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/dial", func(c context.Context) error {
		return sess.Tab().Evaluate(c, strings.ReplaceAll(dial, "PEER", strconv.Quote(peer)), &kicked)
	}); err != nil {
		t.Fatalf("dialling: %v", err)
	}
	var dialOut string
	for i := 0; i < 40; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/dial-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__dial || ""`, &dialOut)
		}); err != nil {
			t.Fatalf("reading the dial: %v", err)
		}
		if dialOut != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("startWAWebVoipCall answered: %s", dialOut)
	t.Log("call placed; watching the collection")

	moved := false
	for i := 0; i < 20; i++ {
		now := read("during")
		if now != before {
			t.Logf("t+%ds MOVED: %s", i, now)
			moved = true
		}
		time.Sleep(time.Second)
	}
	if !moved {
		t.Errorf("NOTHING in the call collection moved in 20s after a placed call.\n"+
			"That is the conclusion H93 was not entitled to and now is: with every "+
			"container watched by name, a dial that produced no change anywhere means "+
			"the dial did nothing — not that the reader cannot count.\n"+
			"BEFORE and DURING both: %s", before)
	}
}
