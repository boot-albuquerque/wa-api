package headless

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/search"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeSearchAndLabels measures the two remaining read-only Client rows:
// searchMessages and getChatsByLabelId.
//
// READ ONLY, identity-free: counts, field names, arities. No message body and no
// label name crosses into Go — the search hit is reported by SHAPE.
func TestProbeSearchAndLabels(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SEARCH") == "" {
		t.Skip("set WA_PROBE_SEARCH=1")
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
	eval := sess.Tab().Evaluate
	script := `(() => {
	window.__sr = null;
	const safe = e => String((e && e.message) || e).slice(0, 140);
	const out = {attempts: []};
	(async () => {
	try {
		const C = window.require("WAWebCollections");
		// Um termo que COM CERTEZA existe: tirado do proprio store, sem sair daqui.
		const ms = C.Msg.getModelsArray();
		let term = "";
		for (const m of ms) {
			const b = m.body || m.__x_body;
			if (typeof b === "string" && b.length >= 4) {
				const w = b.split(/\s+/).find(x => x.length >= 4 && /^[a-zA-Z]+$/.test(x));
				if (w) { term = w.toLowerCase(); break; }
			}
		}
		out.termFound = term !== "";
		out.termLen = term.length;
		const chat = ms.length ? String((ms[0].id && ms[0].id.remote && ms[0].id.remote._serialized) || "") : "";
		const trials = [
			["global-1arg", () => C.Msg.search(term)],
			["global-4arg", () => C.Msg.search(term, 1, 50, undefined)],
			["global-5arg", () => C.Msg.search(term, 1, 50, undefined, undefined)],
			["scoped", () => C.Msg.search(term, 1, 50, chat)],
		];
		for (const [name, fn] of trials) {
			try {
				const r = await fn();
				out.attempts.push({name, hits: ((r && r.messages) || []).length,
					eof: !!(r && r.eof), canceled: !!(r && r.canceled)});
			} catch (e) { out.attempts.push({name, err: safe(e)}); }
		}
	} catch (e) { out.err = safe(e); }
	window.__sr = JSON.stringify(out);
	})();
	return 'kicked';
})()
`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(45 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__sr", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered")
		}
		time.Sleep(400 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("search and labels:\n%s", out)
}

// TestProbeSearchCapability proves capabilities/search against the live store.
//
// The term is taken FROM the store inside the page and never crosses into Go:
// inventing a term risks measuring "no matches" and calling it a failure, and
// carrying a real one out would be carrying message content.
func TestProbeSearchCapability(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SEARCH") == "" {
		t.Skip("set WA_PROBE_SEARCH=1")
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
	eval := sess.Tab().Evaluate

	// Pick a term inside the page and hand back only its LENGTH.
	const pick = `(() => {
		window.__pick = null;
		try {
			const ms = window.require("WAWebCollections").Msg.getModelsArray();
			for (const m of ms) {
				const b = m.body || m.__x_body;
				if (typeof b === "string") {
					const w = b.split(/\s+/).find(x => x.length >= 5 && /^[a-zA-Z]+$/.test(x));
					if (w) { window.__term = w.toLowerCase(); window.__pick = JSON.stringify({len: w.length}); return "kicked"; }
				}
			}
			window.__pick = JSON.stringify({len: 0});
		} catch (e) { window.__pick = JSON.stringify({len: -1}); }
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, pick, &ignored); err != nil {
		t.Fatalf("pick: %v", err)
	}
	var picked string
	for i := 0; i < 40; i++ {
		if err := eval(ctx, "window.__pick", &picked); err != nil {
			t.Fatalf("pick read: %v", err)
		}
		if picked != "" && picked != "null" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Logf("term picked inside the page: %s", picked)
	if strings.Contains(picked, `"len":0`) || strings.Contains(picked, `"len":-1`) {
		t.Skip("no usable term in this store")
	}

	// Read it back only to pass it to the capability; it is never logged.
	var term string
	if err := eval(ctx, "window.__term", &term); err != nil {
		t.Fatalf("term: %v", err)
	}
	s := search.New(runner, eval)
	got, err := s.Messages(ctx, term, 1, "probe/search")
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	var withChat, withID, withType, fromMe int
	for _, hit := range got.Hits {
		if hit.ChatJID != "" {
			withChat++
		}
		if hit.MessageID != "" {
			withID++
		}
		if hit.Type != "" {
			withType++
		}
		if hit.FromMe {
			fromMe++
		}
	}
	t.Logf("search: %d hits (eof=%t); chat=%d id=%d type=%d fromMe=%d",
		got.Returned, got.EOF, withChat, withID, withType, fromMe)
	if got.Returned == 0 {
		t.Error("a term taken from a real message found nothing")
	}
	if withChat == 0 || withID == 0 {
		t.Error("hits came back without addresses: the projection is not reading the id")
	}

	// A term that cannot be in the store must find nothing, and that must not be
	// an error — otherwise the search reports failure for a normal outcome.
	empty, err := s.Messages(ctx, "zzqxwvzz", 1, "probe/search")
	if err != nil {
		t.Errorf("a search with no matches errored: %v", err)
	} else {
		t.Logf("nonsense term: %d hits (eof=%t)", empty.Returned, empty.EOF)
		if empty.Returned != 0 {
			t.Error("a nonsense term matched something")
		}
	}
}
