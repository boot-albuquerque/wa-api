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

// TestProbeHistorySyncBarrier looks for the signal the event bus needs.
//
// A freshly booted session pushes ~2400 historical events through the bus per
// boot, all marked live, because the replay window closes after the FIRST drain
// and the SPA keeps hydrating for minutes afterwards (H93). The instruction was
// to find an EXPLICIT end-of-sync signal rather than invent a rate heuristic.
//
// The module scan found two candidates and this measures both, from boot,
// once a second — because a barrier that is already true when the first read
// happens is not a barrier, it is a constant.
func TestProbeHistorySyncBarrier(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_HISTSYNC") == "" {
		t.Skip("set WA_PROBE_HISTSYNC=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	const read = `JSON.stringify((() => {
		const out = {};
		const t = (name, fn) => { try { out[name] = String(fn()); } catch (e) { out[name] = 'threw'; } };
		// THE GETTERS TAKE THE MODEL, and the first version of this probe called
		// them bare and got three "threw". A getter that throws is not an absent
		// signal; it is a signal asked the wrong way.
		try {
			const PM = window.require('WAWebHistorySyncProgressModel');
			const model = typeof PM.getHistorySyncProgressModel === 'function'
				? PM.getHistorySyncProgressModel() : null;
			out.hasModel = !!model;
			if (model) {
				out.modelFields = Object.keys(model).filter(k => k.indexOf('__x_') === 0);
				const P = window.require('WAWebHistorySyncProgressGetters');
				t('inProgress', () => P.getInProgress(model));
				t('progress', () => P.getProgress(model));
				t('paused', () => P.getPaused(model));
				t('m_progress', () => model.progress);
				t('m_inProgress', () => model.inProgress);
			}
		} catch (e) { out.progressModule = 'threw: ' + String((e && e.message) || e).slice(0, 90); }
		try {
			const U = window.require('WAWebUserPrefsHistorySync');
			t('initialComplete', () => U.getInitialHistorySyncComplete());
			t('lastChunk', () => U.getLastHistorySyncedChunk());
		} catch (e) { out.prefsModule = 'absent'; }
		try {
			const M = window.require('WAWebMsgCollection').MsgCollection;
			out.msgs = typeof M.getModelsArray === 'function' ? M.getModelsArray().length : -1;
		} catch (e) { out.msgs = -1; }
		try {
			const C = window.require('WAWebChatCollection').ChatCollection;
			out.chats = typeof C.getModelsArray === 'function' ? C.getModelsArray().length : -1;
		} catch (e) { out.chats = -1; }
		return out;
	})())`

	// SAMPLED FROM BOOT, once a second. The collections' sizes go with each
	// sample so the barrier can be checked against the thing it is supposed to
	// bound: if the counts are still climbing after it flips, it is the wrong
	// signal.
	for i := 0; i < 45; i++ {
		var raw string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/hist", func(c context.Context) error {
			return sess.Tab().Evaluate(c, read, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		t.Logf("t+%02ds %s", i, raw)
		time.Sleep(time.Second)
	}
}
