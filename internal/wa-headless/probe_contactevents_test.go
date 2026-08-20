package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeContactEventSurface measures what a contact subscription could
// listen to, before onContact is designed.
//
// The message subscription listens for 'add', taken from the reference and
// since verified. Whether the CONTACT collection uses the same vocabulary is a
// separate question: a roster changes mostly by rows being UPDATED (a pushname
// arriving, a picture changing), not by rows being added, so 'add' alone might
// install cleanly and deliver nothing.
//
// Names and counts only.
func TestProbeContactEventSurface(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CONTACTEV") == "" {
		t.Skip("set WA_PROBE_CONTACTEV=1")
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
	// TEN MINUTES, not the boot deadline. The first two runs of this probe
	// passed nCycleReadyDeadline (60s) as the parent and then slept 90s inside
	// it, so every read failed with "deadline of 5s exceeded" — a message that
	// blames the page for a budget the caller had already spent. It cost two
	// wrong conclusions before a control caught it; see HOUSEKEEP H42.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// The event registry is not reachable by name: _events is undefined and the
	// collection's own keys show mangled slots ($1, $2, listenId). Reverse
	// engineering that would be guessing about a minified private field.
	//
	// So the vocabulary is measured EMPIRICALLY instead: bind the catch-all and
	// let a live account tell us which events it really emits. Backbone-style
	// collections pass the event name as the first argument to an 'all'
	// handler, and if this build does not support that, the counter simply
	// stays empty — which is itself an answer, and a distinguishable one.
	// THE CONTROL STAYS, and it is the reason this probe is trustworthy at all:
	// it is what proved the first two conclusions wrong. "The page stopped
	// answering" is only evidence against the binding if the page WOULD have
	// answered otherwise.
	if os.Getenv("WA_PROBE_CONTACTEV_CONTROL") != "" {
		t.Log("CONTROL: waiting 90s with NOTHING bound")
		time.Sleep(90 * time.Second)
		var probe string
		err := runner.Do(ctx, engine.OpStateProbe, "probe/contactev/control", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `JSON.stringify({alive: true, contacts: window.require('WAWebContactCollection').ContactCollection.getModelsArray().length})`, &probe)
		})
		if err != nil {
			t.Fatalf("CONTROL FAILED: the page was already unresponsive without any "+
				"binding, so the timeouts say nothing about the handler: %v", err)
		}
		t.Logf("CONTROL: page answered normally after the same wait: %s", probe)
		return
	}

	// "allSupported: false and nothing observed" is AMBIGUOUS, and the first
	// version of this probe shipped that ambiguity: it could mean the catch-all
	// does not exist on this build, or that the roster simply had a quiet 90
	// seconds. Those need different next steps, so a SELF-TEST separates them —
	// the probe triggers a private event on itself and checks the handler saw
	// it. If the self-test fires and nothing else does, the vocabulary works and
	// the roster was quiet. If the self-test does not fire, 'all' is not a thing
	// here and no amount of waiting would have told us.
	//
	// Named events are bound alongside, because a roster changes mostly by rows
	// being UPDATED, and 'add' — the word the message subscription uses — may
	// simply never fire for contacts.
	const kick = `(() => {
		window.__waHeadlessContactEv = {
			stage: 'listening', coll: {}, model: {}, allSupported: false,
			selfTestSeen: false, named: {}
		};
		const st = window.__waHeadlessContactEv;
		const CC = window.require('WAWebContactCollection').ContactCollection;
		CC.on('all', function (name) {
			st.allSupported = true;
			const k = String(name);
			if (k === 'waHeadlessSelfTest') { st.selfTestSeen = true; return; }
			st.coll[k] = (st.coll[k] || 0) + 1;
		});
		// The self-test: a private name nothing else can emit.
		try { CC.trigger('waHeadlessSelfTest'); } catch (e) { st.selfTestErr = String((e && e.message) || e).slice(0, 120); }

		// Named events, so a build without a catch-all still answers the
		// question that matters: WHICH word does a contact change arrive under?
		for (const name of ['add','change','remove','reset','update','sort','sync','destroy']) {
			try {
				CC.on(name, function () { st.named[name] = (st.named[name] || 0) + 1; });
			} catch (e) { /* a name the build rejects is itself an answer */ }
		}
		try {
			const all = CC.getModelsArray();
			for (let i = 0; i < all.length && i < 8; i++) {
				all[i].on('all', function (name) {
					const k = String(name);
					st.model[k] = (st.model[k] || 0) + 1;
				});
			}
			st.modelsBound = Math.min(all.length, 8);
		} catch (e) { st.modelErr = String((e && e.message) || e).slice(0, 120); }
		return 'listening';
	})()`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/contactev/bind", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &raw)
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	// Read the self-test IMMEDIATELY: it does not need the wait, and reading it
	// first means a build without a catch-all is known before spending 90s.
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/contactev/selftest", func(c context.Context) error {
		return sess.Tab().Evaluate(c, `JSON.stringify({selfTestSeen: !!(window.__waHeadlessContactEv||{}).selfTestSeen, allSupported: !!(window.__waHeadlessContactEv||{}).allSupported, err: (window.__waHeadlessContactEv||{}).selfTestErr || ''})`, &raw)
	}); err != nil {
		t.Fatalf("selftest read: %v", err)
	}
	t.Logf("SELF-TEST (does the catch-all work at all?): %s", raw)

	t.Log("listening for 90s on the live roster")
	time.Sleep(90 * time.Second)

	const script = `JSON.stringify(window.__waHeadlessContactEv || {stage:'missing'})`

	// Read with retries: the page is under a live event load, and a single
	// 5-second probe that loses the race would report "no answer" for a page
	// that simply had not got round to us.
	var readErr error
	for attempt := 0; attempt < 6; attempt++ {
		readErr = runner.Do(ctx, engine.OpStateProbe, "probe/contactev", func(c context.Context) error {
			return sess.Tab().Evaluate(c, script, &raw)
		})
		if readErr == nil {
			break
		}
		t.Logf("read attempt %d: %v", attempt+1, readErr)
		time.Sleep(2 * time.Second)
	}
	if readErr != nil {
		t.Fatalf("probe: %v", readErr)
	}
	t.Logf("events observed: %s", raw)
}
