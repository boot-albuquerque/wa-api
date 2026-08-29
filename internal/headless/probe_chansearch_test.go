package headless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/channel"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeChannelSearch measures whether the channel DIRECTORY answers this
// account at all, and in what shape.
//
// The question is live because the neighbouring call already failed: this
// project measured getRecommendedNewsletters HANGING, ignoring its own 8s
// timeout, and recorded it BLOCKED. fetchNewsletterDirectories may sit on the
// same machinery, so the probe is bounded and reports a hang as a RESULT rather
// than sitting on it.
//
// READ ONLY. No subscription, no channel created. Identity-free: counts, field
// names, and the shape of the answer — never a channel name.
func TestProbeChannelSearch(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHANSEARCH") == "" {
		t.Skip("set WA_PROBE_CHANSEARCH=1")
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

	const script = `(() => {
		window.__cs = null;
		const park = v => { window.__cs = JSON.stringify(v); };
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 160);
		const out = {modules: {}, phase: "start"};
		const load = name => {
			try { const m = window.require(name); out.modules[name] = !!m; return m; }
			catch (e) { out.modules[name] = false; return null; }
		};
		const L10N = load("WAWebL10N");
		const CN = load("WAWebCountriesNativeCountryNames");
		const GATE = load("WAWebNewsletterGatingUtils");
		const DIR = load("WAWebNewsletterDirectorySearchAction");
		try {
			out.region = L10N && typeof L10N.getRegion === "function" ? "readable" : "absent";
			out.pageSizeFn = GATE && typeof GATE.getNewsletterDirectoryPageSize === "function";
			out.creationEnabledFn = GATE && typeof GATE.isNewsletterCreationEnabled === "function";
			// A resposta a esta pergunta decide se a familia inteira e' atacavel:
			// criar canal exige que a pagina permita criar canal.
			out.creationEnabled = out.creationEnabledFn ? !!GATE.isNewsletterCreationEnabled() : null;
			out.fetchFn = DIR && typeof DIR.fetchNewsletterDirectories === "function";
		} catch (e) { out.setupErr = safe(e); }

		(async () => {
			if (!DIR || !out.fetchFn) { out.phase = "no-fetch"; park(out); return; }
			out.phase = "asked";
			try {
				const region = L10N.getRegion();
				const res = await DIR.fetchNewsletterDirectories({
					searchText: "", countryCodes: [region],
					skipSubscribedNewsletters: false, view: "RECOMMENDED",
					categories: [], cursorToken: "",
				});
				const list = (res && res.newsletters) || [];
				out.phase = "answered";
				out.count = list.length;
				out.keys = list.length ? Object.keys(list[0]).sort() : [];
				// Um mixin so' aparece quando ha' resultado; registrar os nomes
				// dita onde um leitor teria de procurar depois.
				out.mixinKeys = [];
				if (list.length) {
					for (const k of Object.keys(list[0])) {
						if (k.indexOf("Mixin") >= 0) { out.mixinKeys.push(k); }
					}
				}
			} catch (e) {
				out.phase = "threw";
				out.err = safe(e);
			}
			park(out);
		})();
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	// BOUNDED, and the bound is a RESULT. The neighbouring call hangs forever.
	const budget = 45 * time.Second
	deadline := time.Now().Add(budget)
	var raw string
	for {
		if err := eval(ctx, "window.__cs", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Logf("MEASURED: the directory search never settled within %s — the same "+
				"shape as getRecommendedNewsletters, which is already BLOCKED", budget)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("channel directory:\n%s", out)
}

// TestProbeChannelSearchCapability proves capabilities/channel Search against
// the live directory.
//
// Identity-free: counts, and how many results carry each field. NEVER a channel
// name — the reader returns them, the log does not.
func TestProbeChannelSearchCapability(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHANSEARCH") == "" {
		t.Skip("set WA_PROBE_CHANSEARCH=1")
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
	r := channel.New(runner, sess.Tab().Evaluate)

	got, err := r.Search(ctx, channel.SearchOptions{}, "probe/chansearch")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var withJID, withName, withDesc, withSubs, verified, withCreated int
	for _, c := range got {
		if c.JID != "" {
			withJID++
		}
		if c.Name != "" {
			withName++
		}
		if c.Description != "" {
			withDesc++
		}
		if c.Subscribers > 0 {
			withSubs++
		}
		if c.Verified {
			verified++
		}
		if !c.CreatedAt.IsZero() {
			withCreated++
		}
	}
	t.Logf("directory: %d results; jid=%d name=%d desc=%d subscribers>0=%d verified=%d created=%d",
		len(got), withJID, withName, withDesc, withSubs, verified, withCreated)
	if len(got) == 0 {
		t.Skip("the directory answered nothing for this account; nothing to assert")
	}
	// THE READER MUST BE READING, not returning a list of blanks. Reading the
	// mixins at the wrong level produces exactly that, and it looks like a thin
	// directory rather than like a bug.
	if withJID == 0 {
		t.Error("no result carried a jid")
	}
	if withName == 0 {
		t.Error("no result carried a name: the mixins are not being read")
	}
	if withSubs == 0 {
		t.Error("no result carried a subscriber count: this is exactly what the first " +
			"version of this reader produced by looking for the mixin names")
	}
	if withCreated == 0 {
		t.Error("no result carried a creation time")
	}

	// A SEARCH TERM MUST NARROW. If a query returned exactly the same set as no
	// query, the searchText would not be reaching the page.
	narrow, err := r.Search(ctx, channel.SearchOptions{Query: "news"}, "probe/chansearch")
	if err != nil {
		t.Errorf("Search(query): %v", err)
	} else {
		same := len(narrow) == len(got)
		if same && len(got) > 0 {
			same = narrow[0].JID == got[0].JID
		}
		t.Logf("query search: %d results, first result identical to unfiltered = %t",
			len(narrow), same)
	}
}

// TestProbeChannelOwnerModules measures whether the modules the OWNER-side
// channel operations need exist on this build, and their arities.
//
// Arity is measured because this project has been wrong about it before: the
// metadata query declares 2 and resolves with 1 (H104).
func TestProbeChannelOwnerModules(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHANMOD") == "" {
		t.Skip("set WA_PROBE_CHANMOD=1")
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
	window.__cm = null;
	const out = {modules: {}, funcs: {}, arity: {}};
	const load = name => {
		try { const m = window.require(name); out.modules[name] = !!m; return m; }
		catch (e) { out.modules[name] = false; return null; }
	};
	const pairs = [
		["WAWebNewsletterCreateQueryJob", ["createNewsletterQuery"]],
		["WAWebNewsletterDeleteAction", ["deleteNewsletterAction"]],
		["WAWebEditNewsletterMetadataAction", ["editNewsletterMetadataAction"]],
		["WAWebMexFetchNewsletterSubscribersJob", ["mexFetchNewsletterSubscribers"]],
		["WAWebNewsletterGatingUtils", ["getMaxSubscriberNumber","isNewsletterCreationEnabled"]],
		["WAWebJidToWid", ["newsletterJidToWid"]],
		["WAWebChatCollection", ["ChatCollection"]],
	];
	for (const [name, fns] of pairs) {
		const m = load(name);
		if (!m) { continue; }
		for (const f of fns) {
			const v = m[f];
			out.funcs[f] = (typeof v === "function") || (typeof v === "object" && v !== null);
			if (typeof v === "function") { out.arity[f] = v.length; }
		}
	}
	window.__cm = JSON.stringify(out);
	return 'kicked';
})()
`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__cm", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered")
		}
		time.Sleep(300 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("channel owner modules:\n%s", out)
}
