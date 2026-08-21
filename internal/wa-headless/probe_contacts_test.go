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

// TestProbeContactShape is a measurement bench, not a guard. Its script is
// rewritten as questions come up; what stays is the harness.
//
// It prints counts and shapes, never a name, a number or an identity.
func TestProbeContactShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CONTACTS") == "" {
		t.Skip("set WA_PROBE_CONTACTS=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// CAN A CHAT LIST SHOW A NAME AT ALL?
	//
	// WAWebChatGetters exports getName, and the obvious move is to promise it.
	// But getName on CONTACTS answered for 1 of 944 on this profile (H39), so
	// assuming chats are different is exactly the guess this project keeps
	// paying for. Counted, never printed.
	const script = `JSON.stringify((() => {
		const out = {};
		const Chats = window.require('WAWebChatCollection').ChatCollection;
		const G = window.require('WAWebChatGetters');
		const all = Chats.getModelsArray();
		out.total = all.length;
		let name = 0, formatted = 0, both = 0, neither = 0, groupNamed = 0, groups = 0;
		for (const c of all) {
			let n = '', f = '';
			try { n = (G.getName && G.getName(c)) || ''; } catch (e) {}
			try { f = (typeof c.formattedTitle === 'string') ? c.formattedTitle : ''; } catch (e) {}
			if (n) { name++; }
			if (f) { formatted++; }
			if (n && f) { both++; }
			if (!n && !f) { neither++; }
			try {
				if (c.id && c.id.server === 'g.us') { groups++; if (n || f) { groupNamed++; } }
			} catch (e) {}
		}
		out.withGetName = name; out.withFormattedTitle = formatted;
		out.withBoth = both; out.withNeither = neither;
		out.groups = groups; out.groupsNamed = groupNamed;
		// Is the collection already ordered by recency, or must we sort?
		const ts = all.map(c => (typeof c.t === 'number' ? c.t : 0));
		let desc = true, asc = true;
		for (let i = 1; i < ts.length; i++) {
			if (ts[i] > ts[i-1]) { desc = false; }
			if (ts[i] < ts[i-1]) { asc = false; }
		}
		out.collectionOrder = desc ? 'newest-first' : (asc ? 'oldest-first' : 'unordered');
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/chats", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("chat naming: %s", raw)
}
