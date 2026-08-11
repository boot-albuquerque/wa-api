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
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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
			fmt.Fprint(w, html)
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
		// No case here may capture text: ready and qr match on structure, and
		// the off-host page is decided by its URL.
		if snap.TextSample != "" {
			t.Errorf("%s: page text was captured into the snapshot: %q", tc.path, snap.TextSample)
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
		fmt.Fprint(w, `<html><body><script>
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
		fmt.Fprint(w, `<html><body><div id="pane-side"></div><script>
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
		fmt.Fprint(w, body)
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
