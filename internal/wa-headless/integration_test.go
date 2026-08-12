package waheadless

// The whole chain against a REAL browser: launch, attach, navigate, probe,
// classify, stop cleanly.
//
// It uses a local page that imitates the three states we classify, never
// web.whatsapp.com. That is not a convenience — a test that reached the real
// target would boot a paired profile, and phase 4C measured that repeated boots
// are exactly how a session degrades. Nothing here touches an account.
//
// What it therefore proves: that the chain works against a browser. What it
// cannot prove: that WhatsApp's real markup still matches our selectors. That
// is CAP-03's remaining verification and it needs a real account.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// chromeCandidates are where a browser usually is. The test SKIPS when none
// exists rather than failing: a machine without Chrome has not broken anything.
var chromeCandidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
	"/usr/bin/chromium",
	"/usr/bin/chromium-browser",
	"/usr/bin/google-chrome",
}

func findChrome(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("WA_HEADLESS_CHROME"); p != "" {
		return p
	}
	for _, p := range chromeCandidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	t.Skip("no Chrome/Chromium found; set WA_HEADLESS_CHROME to run the browser chain")
	return ""
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = l.Close() }()
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return port
}

// pages imitate the three states, by the markers the classifier keys on.
//
// The ready page carries a #pane-side AND chat-list-looking text, because the
// PII claim is exactly that the text of a ready page is never fetched. A double
// with an empty body would let a text-first implementation pass.
const (
	readyPage = `<html><body>
		<div id="pane-side">
			<div>Mum &mdash; see you at 8</div>
			<div>Work &mdash; the deploy is out</div>
		</div></body></html>`
	qrPage = `<html><body>
		<canvas aria-label="Scan this QR code to link a device"></canvas>
		</body></html>`
	conflictPage = `<html><body>
		<p>WhatsApp is open in another window. Click "Use Here" to use it here.</p>
		</body></html>`
	// H6: the application is loaded and our selector no longer matches it.
	//
	// This is not a hypothetical page. It is readyPage with the id changed, which
	// is exactly what a rename by Meta looks like from in here: same chat list,
	// same text, and a structural probe that now finds nothing.
	renamedPanePage = `<html><body>
		<div id="pane-side-v2">
			<div>Mum &mdash; see you at 8</div>
			<div>Work &mdash; the deploy is out</div>
			<div>+55 11 99999-0000 &mdash; are we still on?</div>
		</div></body></html>`
	// The same rename, on a page that ALSO carries the conflict wording. It
	// proves the guard did not buy privacy by giving up the class.
	renamedPaneConflictPage = `<html><body>
		<div id="pane-side-v2">
			<div>Mum &mdash; see you at 8</div>
		</div>
		<p>WhatsApp is open in another window. Click "Use Here" to use it here.</p>
		</body></html>`
)

func pageServer(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range map[string]string{
		"/ready":    readyPage,
		"/qr":       qrPage,
		"/conflict": conflictPage,
	} {
		html := body
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, html)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestBrowserChainLaunchesNavigatesAndClassifies(t *testing.T) {
	binary := findChrome(t)
	base := pageServer(t)

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}

	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir:    t.TempDir(),
		DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	var stoppedVia engine.StopVia
	defer func() {
		stoppedVia = engine.CleanStop(context.Background(), runner, browser)
		if !stoppedVia.Clean() {
			t.Errorf("stopped_via=%s: a browser this test owns must go down through "+
				"the protocol", stoppedVia)
		}
	}()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	// The conflict screen is NOT asserted here, and the reason is worth stating:
	// these pages are served from 127.0.0.1, so the classifier reaches REDIRECT
	// — correctly — before it ever looks at text. Reproducing SESSION_CONFLICT
	// would need the page to be on web.whatsapp.com, which this test refuses to
	// touch. It is covered at unit level, where the URL can be whatever the case
	// requires.
	//
	// The off-host page is asserted for what it actually is, and that is a
	// behaviour worth pinning: an off-host page is classified WITHOUT its text
	// being read.
	cases := []struct {
		path string
		want spa.PageClass
	}{
		{"/ready", spa.ClassAppReady},
		{"/qr", spa.ClassLoginRequired},
		{"/conflict", spa.ClassRedirect},
	}
	for _, tc := range cases {
		if err := tab.Navigate(runner, base+tc.path, "nav"+tc.path); err != nil {
			t.Fatalf("Navigate %s: %v", tc.path, err)
		}
		snap, got := spa.Probe(context.Background(), runner, tab.Evaluate, "probe"+tc.path)
		if got != tc.want {
			t.Errorf("%s classified as %q, want %q (snapshot %+v)", tc.path, got, tc.want, snap)
		}
		// No case here may report markers: ready and qr match on structure, and
		// the off-host page is decided by its URL.
		if len(snap.Markers) != 0 {
			t.Errorf("%s: markers were gathered from a page decided without text: %v",
				tc.path, snap.Markers)
		}
	}
}

// A page that stops executing JavaScript must come back UNRESPONSIVE, against a
// real browser and not only against a double. This is the phase 6 state,
// reproduced on purpose: the target exists, the process is alive, and nothing
// structural says a thing is wrong.
func TestBrowserChainReportsAWedgedPageAsUnresponsive(t *testing.T) {
	binary := findChrome(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// A synchronous spin with no exit: the main thread never returns to the
		// event loop, so no evaluation can be scheduled.
		_, _ = fmt.Fprint(w, `<html><body><script>
			window.addEventListener('load', function () { for (;;) {} });
		</script></body></html>`)
	}))
	defer srv.Close()

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}

	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir:    t.TempDir(),
		DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() {
		// A wedged renderer will not honour Browser.close, so the dirty path is
		// the expected outcome here. What matters is that it terminates.
		via := engine.CleanStop(context.Background(), runner, browser)
		t.Logf("stopped_via=%s", via)
	}()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	// Navigate itself may not return on a page that wedges during load; its own
	// budget covers that, and either outcome leaves the page wedged.
	_ = tab.Navigate(runner, srv.URL, "nav/wedged")

	start := time.Now()
	_, got := spa.Probe(context.Background(), runner, tab.Evaluate, "probe/wedged")
	elapsed := time.Since(start)

	if got != spa.ClassUnresponsive {
		t.Fatalf("a page spinning forever classified as %q, want %q", got, spa.ClassUnresponsive)
	}
	// The budget is what makes it terminate. Without a Go-side deadline this is
	// the 24-minute hang of phase 6.
	if elapsed > 30*time.Second {
		t.Fatalf("the probe took %v; the StateProbe budget is %v", elapsed, engine.DefaultDeadlines.StateProbe)
	}
}

// Liveness against a REAL browser, in the two states that matter.
//
// The unit tests pin the streak arithmetic and the classification; what they
// cannot show is that the probe survives a real renderer. This does — and the
// wedged half reproduces the phase 6 state, where every structural signal says
// healthy and only a deadlined evaluation disagrees.
func TestBrowserChainLivenessSeesAliveAndWedged(t *testing.T) {
	binary := findChrome(t)
	base := pageServer(t)

	wedged := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The application mounts, THEN the main thread stops returning. This is
		// the shape that fools a structural check: #pane-side is in the DOM.
		_, _ = fmt.Fprint(w, `<html><body><div id="pane-side"></div><script>
			window.addEventListener('load', function () { for (;;) {} });
		</script></body></html>`)
	}))
	defer wedged.Close()

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}

	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir:    t.TempDir(),
		DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() { t.Logf("stopped_via=%s", engine.CleanStop(context.Background(), runner, browser)) }()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	monitor := &spa.Monitor{Runner: runner, Eval: tab.Evaluate, UnresponsiveAfter: 2}

	if err := tab.Navigate(runner, base+"/ready", "nav/ready"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	live := monitor.Check(context.Background(), "live/ready")
	if !live.Alive {
		t.Fatalf("a mounted application probed as not alive: class=%q err=%v", live.Class, live.Err)
	}
	if live.Latency <= 0 {
		t.Error("no latency recorded against a real browser")
	}

	// Same tab, same browser, same process — only the page changes.
	_ = tab.Navigate(runner, wedged.URL, "nav/wedged")

	var res spa.LivenessResult
	for i := 0; i < monitor.UnresponsiveAfter; i++ {
		res = monitor.Check(context.Background(), "live/wedged")
	}
	if !res.Unresponsive() {
		t.Fatalf("a wedged page with #pane-side in the DOM probed as %q; a structural "+
			"check would have called this session healthy", res.Class)
	}

	// The process is still up and the target still attached. That is the whole
	// finding: nothing outside the evaluation knows anything is wrong.
	if browser.PID() <= 0 {
		t.Error("the browser process died; this test would then prove nothing")
	}
	if !engine.ProcessAlive(browser.PID()) {
		t.Error("the browser process is gone, so UNRESPONSIVE could have come from " +
			"the browser dying rather than from the page wedging")
	}
}

// requirePage serves a page whose window.require knows exactly `known`.
//
// It throws for anything else, which is what the real one does — and that is
// the behaviour the resolve script has to survive. A double that returned
// undefined instead of throwing would let a script with no try/catch pass,
// and against the real page the first missing module would abort the whole
// check and report the rest as fine.
func requirePage(t *testing.T, known []spa.Module) string {
	t.Helper()
	names := make([]string, len(known))
	for i, m := range known {
		names[i] = `"` + string(m) + `"`
	}
	body := `<html><body><div id="pane-side"></div><script>
		const known = new Set([` + strings.Join(names, ",") + `]);
		window.require = function (name) {
			if (!known.has(name)) { throw new Error("Cannot find module '" + name + "'"); }
			return { __module: name };
		};
	</script></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// The requirement of ADR-0006 D4 against a real JavaScript engine: the
// inventory passes when the page exposes what we need, and renaming ONE module
// stops the boot with a message that names it.
func TestBrowserChainVerifiesTheModuleInventory(t *testing.T) {
	binary := findChrome(t)

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}
	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir:    t.TempDir(),
		DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() { t.Logf("stopped_via=%s", engine.CleanStop(context.Background(), runner, browser)) }()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	// Every module present.
	if err := tab.Navigate(runner, requirePage(t, spa.RequiredAtStartup), "nav/complete"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if err := spa.VerifyInventory(context.Background(), runner, tab.Evaluate, spa.RequiredAtStartup); err != nil {
		t.Fatalf("a page exposing every module failed the inventory: %v", err)
	}

	// Meta renames one. This is the negative control the ADR asks for, run
	// against a real engine rather than a string.
	renamed := make([]spa.Module, len(spa.RequiredAtStartup))
	copy(renamed, spa.RequiredAtStartup)
	renamed[2] = spa.Module(string(renamed[2]) + "V2")

	if err := tab.Navigate(runner, requirePage(t, renamed), "nav/renamed"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	err = spa.VerifyInventory(context.Background(), runner, tab.Evaluate, spa.RequiredAtStartup)

	var missing *spa.ErrModulesMissing
	if !errors.As(err, &missing) {
		t.Fatalf("a renamed module did not stop the boot: %v", err)
	}
	if len(missing.Missing) != 1 || missing.Missing[0] != spa.RequiredAtStartup[2] {
		t.Fatalf("reported %v missing, want exactly %s — the check must name the "+
			"module that moved, not the ones that did not", missing.Missing, spa.RequiredAtStartup[2])
	}
	if !strings.Contains(err.Error(), string(spa.RequiredAtStartup[2])) {
		t.Errorf("the message does not name the missing module: %v", err)
	}
}

// The severing instrument, proved against a real browser before it is pointed
// at an account.
//
// LOOP 04.3A needs to produce a session whose UI is mounted and whose server is
// gone. If the emulation did not actually sever, every signal would stay put and
// the run would be reported as "nothing changed while offline" — the instrument
// inventing its own finding, which is the failure this repository keeps paying
// for. So the sever is measured HERE, where the other end of the connection is a
// local server this test owns and can see.
//
// Both halves are asserted, and the online halves are the control: a "fetch
// failed" that also fails while online would prove nothing about the sever.
func TestBrowserChainSeversAndRestoresThePageNetwork(t *testing.T) {
	binary := findChrome(t)

	// Atomic because the counter is written by the server's goroutine and read
	// by the test's: the requests are ordered in time, the memory accesses are
	// not.
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No-store, because a cached answer would come back with the network
		// severed and read as "the sever did not work".
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/ping" {
			hits.Add(1)
			_, _ = fmt.Fprint(w, "pong")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, readyPage)
	}))
	defer srv.Close()

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}
	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() { t.Logf("stopped_via=%s", engine.CleanStop(context.Background(), runner, browser)) }()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	if err := tab.Navigate(runner, srv.URL+"/", "nav/sever"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	steps := []severStep{
		// The first leg emulates nothing: it is the state the browser launched
		// in, and it is what makes the severed leg's failure attributable.
		{name: "before", emulate: false, wantOnline: true, wantFetch: fetchVerdictOK},
		{name: "severed", emulate: true, offline: true, wantOnline: false, wantFetch: fetchVerdictFail},
		{name: "restored", emulate: true, offline: false, wantOnline: true, wantFetch: fetchVerdictOK},
	}
	var hitsBeforeSever int64
	for _, step := range steps {
		if step.offline {
			hitsBeforeSever = hits.Load()
		}
		checkSeverStep(t, runner, tab, srv.URL, step)
	}

	// The request must not have reached the server at all while severed. A fetch
	// that failed AFTER being served would be a broken response, not a broken
	// network, and the two are different measurements.
	if served := hits.Load() - hitsBeforeSever; served != 1 {
		t.Errorf("the server saw %d requests after the sever, want exactly 1 (the "+
			"restored leg): a request arrived while the page was supposed to be "+
			"severed", served)
	}
}

// severStep is one leg of the sever test: what to emulate, and what the page
// must then report.
type severStep struct {
	name string
	// emulate distinguishes "leave the browser as it launched" from "ask for
	// offline=false". Both look online; only the second exercises the restore.
	emulate    bool
	offline    bool
	wantOnline bool
	wantFetch  string
}

func checkSeverStep(t *testing.T, runner *engine.Runner, tab *engine.Tab, baseURL string, step severStep) {
	t.Helper()
	if step.emulate {
		if err := tab.SetNetworkOffline(runner, step.offline, "sever/"+step.name); err != nil {
			t.Fatalf("%s: SetNetworkOffline(%v): %v", step.name, step.offline, err)
		}
	}
	if online := navigatorOnline(t, runner, tab, step.name); online != step.wantOnline {
		t.Errorf("%s: navigator.onLine=%v, want %v — the emulation did not reach "+
			"the application's view of the network", step.name, online, step.wantOnline)
	}
	if got := fetchVerdict(t, runner, tab, baseURL+"/ping?"+step.name); got != step.wantFetch {
		t.Errorf("%s: fetch verdict %q, want %q", step.name, got, step.wantFetch)
	}
}

// fetchVerdict values. The page reports a WORD, never a response body.
const (
	fetchVerdictPending = "PENDING"
	fetchVerdictOK      = "OK"
	fetchVerdictFail    = "FAIL"
)

// fetchProbeSlot is where the page parks its verdict between the two
// evaluations. Named once, because a literal repeated across two scripts is the
// same bug waiting to diverge.
const fetchProbeSlot = "__waHeadlessFetchVerdict"

// fetchVerdict starts a fetch in the page and waits, from the GO side, for it to
// settle.
//
// The wait is a Go loop on purpose. A promise awaited inside the page would put
// the clock where a stalled renderer can stop it, which is exactly what the
// module's gate forbids.
func fetchVerdict(t *testing.T, runner *engine.Runner, tab *engine.Tab, url string) string {
	t.Helper()

	start := `(() => { window.` + fetchProbeSlot + ` = '` + fetchVerdictPending + `';` +
		`fetch(` + strconv.Quote(url) + `, {cache: 'no-store'})` +
		`.then(() => { window.` + fetchProbeSlot + ` = '` + fetchVerdictOK + `'; })` +
		`.catch(() => { window.` + fetchProbeSlot + ` = '` + fetchVerdictFail + `'; });` +
		`return JSON.stringify('started'); })()`
	var raw string
	if err := runner.Do(context.Background(), engine.OpEvaluate, "fetch/start",
		func(ctx context.Context) error { return tab.Evaluate(ctx, start, &raw) }); err != nil {
		t.Fatalf("starting the fetch probe: %v", err)
	}

	read := `JSON.stringify(window.` + fetchProbeSlot + ` || '` + fetchVerdictPending + `')`
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if err := runner.Do(context.Background(), engine.OpStateProbe, "fetch/read",
			func(ctx context.Context) error { return tab.Evaluate(ctx, read, &raw) }); err != nil {
			t.Fatalf("reading the fetch verdict: %v", err)
		}
		var verdict string
		if err := json.Unmarshal([]byte(raw), &verdict); err != nil {
			t.Fatalf("unexpected fetch verdict shape %q: %v", raw, err)
		}
		if verdict != fetchVerdictPending {
			return verdict
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fetchVerdictPending
}

func navigatorOnline(t *testing.T, runner *engine.Runner, tab *engine.Tab, label string) bool {
	t.Helper()
	var raw string
	if err := runner.Do(context.Background(), engine.OpStateProbe, "online/"+label,
		func(ctx context.Context) error {
			return tab.Evaluate(ctx, `JSON.stringify(navigator.onLine)`, &raw)
		}); err != nil {
		t.Fatalf("%s: reading navigator.onLine: %v", label, err)
	}
	var online bool
	if err := json.Unmarshal([]byte(raw), &online); err != nil {
		t.Fatalf("%s: unexpected navigator.onLine shape %q: %v", label, raw, err)
	}
	return online
}

// The sever, proved against the transport the finding actually rests on.
//
// The test above proves the emulation against an HTTP fetch. The signal M4
// measured is not a fetch: it is the SPA's WebSocket. A Chrome that applied the
// emulation to fetch but let an already-open WebSocket keep carrying frames
// would turn every conclusion of M4 into a statement about our instrument, and
// nothing in the fetch test would notice. So the WebSocket is asserted here,
// against a server this test owns and can count.
//
// It measures DELIVERY, in both directions, and never intent. The page keeps
// calling send() throughout the cut — its readyState never leaves OPEN, which
// is itself the measured behaviour — so "the page sent frames" proves nothing
// at all. What proves the cut is that the SERVER stops receiving them and the
// PAGE stops receiving the echoes, while the page is demonstrably still trying.
//
// Both cut shapes are covered, because both are load-bearing: the full sever is
// what M4 used, and the transport-only one is what LOOP 04.3B's precondition
// relies on when it infers a dead socket from a dead fetch.
func TestBrowserChainSeversTheWebSocketTransport(t *testing.T) {
	binary := findChrome(t)

	pageURL, serverFrames := wsEchoServer(t)

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}
	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() { t.Logf("stopped_via=%s", engine.CleanStop(context.Background(), runner, browser)) }()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	// Method expressions, so the two cut shapes differ by exactly the command
	// they send and by nothing else in the leg around them.
	for _, cut := range []struct {
		name string
		set  func(*engine.Tab, *engine.Runner, bool, string) error
	}{
		{"full", (*engine.Tab).SetNetworkOffline},
		{"transport-only", (*engine.Tab).SetTransportOffline},
	} {
		t.Run(cut.name, func(t *testing.T) {
			severWebSocketLeg(t, runner, tab, pageURL, serverFrames, cut.name, cut.set)
		})
	}
}

// severWebSocketLeg is one cut shape, watched online, severed and restored.
func severWebSocketLeg(t *testing.T, runner *engine.Runner, tab *engine.Tab, pageURL string,
	serverFrames *atomic.Int64, name string,
	set func(*engine.Tab, *engine.Runner, bool, string) error) {
	t.Helper()

	// A fresh navigation per leg, so each leg opens its own socket and starts
	// its counters from zero.
	if err := tab.Navigate(runner, pageURL, "nav/ws/"+name); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	awaitWSOpen(t, runner, tab, name)

	// ONLINE: the control. Frames must flow, or a later "zero frames" would say
	// nothing about the cut.
	before := wsDelta(t, runner, tab, serverFrames, name+"/online", wsPhase)
	if before.served == 0 || before.received == 0 {
		t.Fatalf("%s: with the network untouched the server saw %d frames and the page "+
			"received %d; the counters do not work, so nothing below could be attributed "+
			"to a cut", name, before.served, before.received)
	}

	if err := set(tab, runner, true, "ws-sever/"+name); err != nil {
		t.Fatalf("%s: severing: %v", name, err)
	}
	t.Cleanup(func() {
		if err := set(tab, runner, false, "ws-restore/"+name); err != nil {
			t.Errorf("%s: restoring the network: %v", name, err)
		}
	})

	during := wsDelta(t, runner, tab, serverFrames, name+"/severed", wsPhase)
	assertNothingDelivered(t, name, during)
	t.Logf("%s: severed window — page sent %d, server received %d, page received %d, "+
		"readyState=%d closed=%v", name, during.sent, during.served,
		during.received, during.readyState, during.closed)

	if err := set(tab, runner, false, "ws-restore/"+name); err != nil {
		t.Fatalf("%s: restoring: %v", name, err)
	}
	after := wsDelta(t, runner, tab, serverFrames, name+"/restored", wsPhase)
	if after.served == 0 || after.received == 0 {
		t.Errorf("%s: after the restore the server saw %d frames and the page received "+
			"%d; the emulation was not undone", name, after.served, after.received)
	}
}

// assertNothingDelivered is the assertion the whole test exists for: with the
// page still handing frames to an OPEN socket, neither end received any.
func assertNothingDelivered(t *testing.T, name string, during wsWindow) {
	t.Helper()
	if during.sent == 0 {
		t.Fatalf("%s: the page stopped calling send() during the cut, so zero deliveries "+
			"would mean a stopped producer and not a severed transport", name)
	}
	if during.served != 0 {
		t.Errorf("%s: the server received %d frames while the page was supposed to be "+
			"severed; the emulation does not reach an open WebSocket", name, during.served)
	}
	if during.received != 0 {
		t.Errorf("%s: the page received %d frames while severed; the emulation does not "+
			"reach an open WebSocket", name, during.received)
	}
}

// wsPhase is how long each leg of the WebSocket test is watched. At the page's
// 200 ms send interval it is about ten frames, which is enough for "some" and
// "none" to be different by a wide margin rather than by one tick.
const wsPhase = 2 * time.Second

// wsProbeSlot is where the page keeps its frame counters. Named once: a literal
// repeated between the page script and the reader is the same bug waiting to
// diverge.
const wsProbeSlot = "__waHeadlessWS"

// wsProbePage opens a WebSocket and reports COUNTS — never a frame's contents.
// The payload it sends is a fixed character, so nothing about this page's
// traffic depends on anything a person wrote.
//
// The send loop is unconditional on the emulation: it keeps calling send() for
// as long as readyState says OPEN, which is what makes the severed window's
// zero deliveries attributable to the transport rather than to a page that
// gave up.
const wsProbePage = `<html><body><script>
window.` + wsProbeSlot + ` = {open: false, sent: 0, received: 0, closed: false, ready_state: -1};
(function () {
	var st = window.` + wsProbeSlot + `;
	var s = new WebSocket(%s);
	s.onopen = function () { st.open = true; };
	s.onmessage = function () { st.received++; };
	s.onclose = function () { st.closed = true; };
	setInterval(function () {
		st.ready_state = s.readyState;
		if (s.readyState === 1) { s.send('x'); st.sent++; }
	}, 200);
})();
</script></body></html>`

// wsCounts is the page's side of the ledger.
type wsCounts struct {
	Open       bool `json:"open"`
	Sent       int  `json:"sent"`
	Received   int  `json:"received"`
	Closed     bool `json:"closed"`
	ReadyState int  `json:"ready_state"`
}

// wsWindow is what one watched phase delivered: the page's two counters and the
// server's, all as DELTAS over the phase.
type wsWindow struct {
	sent, received, served int
	readyState             int
	closed                 bool
}

// wsEchoServer serves the probe page and echoes every frame back, counting what
// arrives.
func wsEchoServer(t *testing.T) (pageURL string, frames *atomic.Int64) {
	t.Helper()
	// Atomic because the counter is written by the server's goroutines and read
	// by the test's.
	var served atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		for {
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			served.Add(1)
			if err := conn.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The socket URL is built from the request's own Host, which is what
		// keeps the page same-origin with the server that accepts it.
		_, _ = fmt.Fprintf(w, wsProbePage, strconv.Quote("ws://"+r.Host+"/ws"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/", &served
}

// awaitWSOpen waits, from the GO side, for the page's socket to connect. The
// clock stays out of the page for the same reason the fetch probe's does.
func awaitWSOpen(t *testing.T, runner *engine.Runner, tab *engine.Tab, label string) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		if readWSCounts(t, runner, tab, label).Open {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s: the page's WebSocket never opened", label)
}

// wsDelta watches for one phase and returns what moved during it.
func wsDelta(t *testing.T, runner *engine.Runner, tab *engine.Tab,
	served *atomic.Int64, label string, phase time.Duration) wsWindow {
	t.Helper()
	start := readWSCounts(t, runner, tab, label+"/start")
	servedStart := served.Load()
	time.Sleep(phase)
	end := readWSCounts(t, runner, tab, label+"/end")
	return wsWindow{
		sent:       end.Sent - start.Sent,
		received:   end.Received - start.Received,
		served:     int(served.Load() - servedStart),
		readyState: end.ReadyState,
		closed:     end.Closed,
	}
}

func readWSCounts(t *testing.T, runner *engine.Runner, tab *engine.Tab, label string) wsCounts {
	t.Helper()
	var raw string
	if err := runner.Do(context.Background(), engine.OpStateProbe, "ws/"+label,
		func(ctx context.Context) error {
			return tab.Evaluate(ctx, `JSON.stringify(window.`+wsProbeSlot+`)`, &raw)
		}); err != nil {
		t.Fatalf("%s: reading the WebSocket counters: %v", label, err)
	}
	var counts wsCounts
	if err := json.Unmarshal([]byte(raw), &counts); err != nil {
		t.Fatalf("%s: unexpected WebSocket counter shape %q: %v", label, raw, err)
	}
	return counts
}

// qrFixture builds the pairing screen as MEASURED on the real SPA
// (EVIDENCIA-SPA.md M1), with each marker independently switchable.
//
// The markup is copied from what was observed, not from what wwebjs expects:
// the canvas sits inside a DIV, the aria-label is the exact English string, and
// the alt-linking markers are the ones that show up six seconds before the code
// does. A fixture that only carried the selector we already match would prove
// nothing about the selector we might need.
func qrFixture(t *testing.T, withTestID, withAria, withLoading bool) string {
	t.Helper()
	body := `<html><body><div id="app">`
	if withLoading {
		body += `<div data-testid="link-device-qrcode-alt-linking-help"></div>
			<div data-testid="link-device-qrcode-alt-linking-hint"></div>
			<div data-testid="loading-spinner"></div>`
	}
	if withTestID || withAria {
		attrs := ""
		if withTestID {
			attrs += ` data-testid="link-device-qr-code"`
		}
		aria := ""
		if withAria {
			aria = ` aria-label="Scan this QR code to link a device!"`
		}
		// [data-ref] carries the QR payload on the real page. The fixture keeps
		// the ATTRIBUTE so the shape matches, with an obviously fake value.
		body += `<div` + attrs + ` data-ref="FIXTURE-NOT-A-REAL-CODE"><canvas` + aria + `></canvas></div>`
	}
	body += `</div></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// The selector work of LOOP B1.2, run in a real JavaScript engine.
//
// A unit test cannot make this claim: the double returns the snapshot fields
// directly, so it never executes a selector. Only a browser can say whether
// `[data-testid="link-device-qr-code"]` matches the markup we measured.
func TestBrowserChainDetectsTheQRByEitherSelector(t *testing.T) {
	binary := findChrome(t)

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}
	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() { t.Logf("stopped_via=%s", engine.CleanStop(context.Background(), runner, browser)) }()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	cases := []struct {
		name                  string
		testID, aria, loading bool
		want                  spa.PageClass
	}{
		// Both, as the real page has them at t+15s.
		{"as measured", true, true, true, spa.ClassLoginRequired},
		// The locale case: an account in Portuguese keeps the hook and loses
		// the English aria-label.
		{"testid only (other locale)", true, false, true, spa.ClassLoginRequired},
		// The wwebjs case: if Meta drops the testid, the aria still carries it.
		{"aria only", false, true, true, spa.ClassLoginRequired},
		// The measured gap at t+9s: pairing screen mounted, no code yet.
		{"pairing screen, no code", false, false, true, spa.ClassPairingLoading},
	}
	for _, tc := range cases {
		url := qrFixture(t, tc.testID, tc.aria, tc.loading)
		if err := tab.Navigate(runner, url, "nav/"+tc.name); err != nil {
			t.Fatalf("%s: Navigate: %v", tc.name, err)
		}
		snap, got := spa.Probe(context.Background(), runner, tab.Evaluate, "probe/"+tc.name)
		if got != tc.want {
			t.Errorf("%s: classified as %q, want %q (snapshot %+v)", tc.name, got, tc.want, snap)
		}
		// The QR payload is a credential. No case may scan page text.
		if len(snap.Markers) != 0 {
			t.Errorf("%s: markers were gathered from a page structure classified: %v",
				tc.name, snap.Markers)
		}
	}
}

// h6Server serves the renamed-pane fixtures under a path that satisfies the
// classifier's own host rule.
//
// The rule is `strings.Contains(lower(URL), "web.whatsapp.com")`, and it is
// checked BEFORE the marker probe: on a plain 127.0.0.1 URL every fixture
// classifies REDIRECT and the text path is never reached — which is why the
// older browser test could only assert the conflict screen at unit level.
//
// Putting the host in the PATH is not a trick played on the classifier; it is
// the classifier's rule, exercised exactly as written, without this test
// resolving or contacting web.whatsapp.com. If somebody tightens that rule to
// parse the host properly, this fixture stops matching and the test fails
// loudly rather than silently skipping the path it exists to cover.
func h6Server(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range map[string]string{
		"/web.whatsapp.com/renamed":          renamedPanePage,
		"/web.whatsapp.com/renamed-conflict": renamedPaneConflictPage,
	} {
		html := body
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, html)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// H6 against a real JavaScript engine: the marker script, executing.
//
// The unit tests hold the Go side, but the guard now LIVES in the page — the
// body text is read, lowercased and discarded inside the browser — and a double
// cannot make that claim. Only a browser can say whether what comes back over
// CDP is a list of our own strings rather than somebody's conversation.
//
// Both fixtures have the application loaded with our selector renamed. That is
// the H6 state: structure matches nothing, so the second probe runs against a
// chat list.
func TestBrowserChainCarriesNoPageTextWhenTheSelectorIsRenamed(t *testing.T) {
	binary := findChrome(t)
	base := h6Server(t)

	runner := engine.NewRunner()
	launcher := &engine.Launcher{BinaryPath: binary, Runner: runner}
	browser, err := launcher.Launch(context.Background(), engine.LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: freePort(t),
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer func() {
		via := engine.CleanStop(context.Background(), runner, browser)
		t.Logf("stopped_via=%s", via)
		if !via.Clean() {
			t.Errorf("stopped_via=%s: a browser this test owns must go down through "+
				"the protocol", via)
		}
	}()

	tab, err := engine.OpenTab(context.Background(), browser)
	if err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	defer tab.Close()

	// The strings a leak would carry. They are in both fixtures' bodies.
	private := []string{"Mum", "see you at 8", "the deploy is out", "99999-0000"}

	cases := []struct {
		name, path string
		want       spa.PageClass
	}{
		// The chat list alone matches none of our vocabulary.
		{"renamed selector, chat list", "/web.whatsapp.com/renamed", spa.ClassOther},
		// And the class we would have lost by gating on identity instead: the
		// conflict screen belongs to a PAIRED profile by definition, so a guard
		// keyed on "no owner identity" would refuse to read exactly here.
		{"renamed selector, conflict wording", "/web.whatsapp.com/renamed-conflict", spa.ClassSessionConflict},
	}
	for _, tc := range cases {
		if err := tab.Navigate(runner, base+tc.path, "nav/"+tc.name); err != nil {
			t.Fatalf("%s: Navigate: %v", tc.name, err)
		}
		snap, got := spa.Probe(context.Background(), runner, tab.Evaluate, "probe/"+tc.name)

		if got != tc.want {
			t.Errorf("%s: classified as %q, want %q (snapshot %+v)", tc.name, got, tc.want, snap)
		}
		// The page HAS text and the structural probe found nothing, so the
		// marker path ran. If it had not, this assertion would be vacuous.
		if snap.TextLength == 0 {
			t.Errorf("%s: the fixture reported no text at all; the marker path was "+
				"not exercised and this case proves nothing", tc.name)
		}
		rendered := fmt.Sprintf("%+v", snap)
		for _, secret := range private {
			if strings.Contains(rendered, secret) {
				t.Errorf("%s: page text crossed the boundary: %q is in %s",
					tc.name, secret, rendered)
			}
		}
	}
}
