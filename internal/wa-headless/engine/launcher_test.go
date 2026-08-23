package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeChromium is a script that ignores the flag set and stays alive, so the
// launcher's real command line is exercised without a browser.
//
// It records the fact that it ran, which is how "the launcher refused BEFORE
// starting anything" is told apart from "the launcher started something and it
// failed" — the difference matters, because the second leaves a process holding
// the profile lock.
func fakeChromium(t *testing.T, startedMarker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-chromium")
	script := "#!/bin/sh\ntouch " + startedMarker + "\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake chromium: %v", err)
	}
	return path
}

// publishingChromium is the fake browser that PUBLISHES its endpoint the way
// the real one does: two lines in <ProfileDir>/DevToolsActivePort, the bound
// port then the browser ws path.
//
// The format is not invented. It was measured against Google Chrome
// 151.0.7922.170 on macOS 15.6 with a throwaway profile (F102):
//
//	55077
//	/devtools/browser/954157ae-38a1-4ef0-b7d5-f03f3c1f7d03
//
// A double that published some other shape would prove only that the parser
// reads what the double writes — the trap this repository has hit before.
func publishingChromium(t *testing.T, startedMarker, profileDir string, port int, wsPath string) string {
	t.Helper()
	return writeFakeScript(t, "#!/bin/sh\ntouch "+startedMarker+"\n"+
		publishLine(profileDir, port, wsPath)+"exec sleep 30\n")
}

// halfWrittenThenCompleteChromium writes the file INCOMPLETE first — only the
// port line — and completes it a moment later.
//
// This is the real race, and it replaces the old "endpoint answers with an
// empty ws URL" double: the browser writes the file in one go, but a reader can
// still arrive mid-write. Treating that as a hard failure would turn an
// ordinary race into a boot failure.
func halfWrittenThenCompleteChromium(t *testing.T, profileDir string, port int, wsPath string) string {
	t.Helper()
	file := filepath.Join(profileDir, activePortFile)
	return writeFakeScript(t, "#!/bin/sh\n"+
		fmt.Sprintf("printf '%%d\\n' %d > %q\n", port, file)+
		"sleep 0.4\n"+
		publishLine(profileDir, port, wsPath)+
		"exec sleep 30\n")
}

func publishLine(profileDir string, port int, wsPath string) string {
	return fmt.Sprintf("printf '%%d\\n%%s\\n' %d %q > %q\n",
		port, wsPath, filepath.Join(profileDir, activePortFile))
}

func writeFakeScript(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-chromium")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake chromium: %v", err)
	}
	return path
}

// devToolsServer answers /json/version the way Chromium does.
func devToolsServer(t *testing.T, wsURL func() string) (port int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != versionEndpoint {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"Browser":"HeadlessChrome/151.0.0.0","webSocketDebuggerUrl":%q}`, wsURL())
	}))
	t.Cleanup(srv.Close)

	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener addr: %v", err)
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return port
}

func shortBootRunner(boot time.Duration) *Runner {
	p := DefaultDeadlines
	p.Boot = boot
	p.Shutdown = 200 * time.Millisecond
	return &Runner{Policy: p, Log: NewRunner().Log}
}

// awaitMarker polls for the fake browser's "I ran" file.
//
// It is polled, not stat'ed once: the DevTools double answers immediately, so
// Launch can return before the script has finished touching the file. A single
// Stat here would be a flake blaming production for a race in the test.
func awaitMarker(t *testing.T, path string) bool {
	t.Helper()
	return awaitMarkerFor(t, path, 5*time.Second)
}

// awaitMarkerFor is the same wait with an explicit window, so that "it never
// appeared" can be asserted as SUSTAINED absence.
//
// A single Stat cannot say that: the script touches the file asynchronously, so
// "not there yet" and "never ran" read identically for the first few
// milliseconds — and the assertion that the launcher refused BEFORE starting
// anything is exactly the one that would pass by luck.
func awaitMarkerFor(t *testing.T, path string, window time.Duration) bool {
	t.Helper()
	for until := time.Now().Add(window); time.Now().Before(until); {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func reapBrowser(t *testing.T, b *Browser) {
	t.Helper()
	t.Cleanup(func() {
		if b != nil && b.PID() > 0 {
			_ = syscall.Kill(-b.PID(), syscall.SIGKILL)
		}
	})
}

func TestLaunchReturnsABrowserOnceTheEndpointAnswers(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	profile := t.TempDir()
	const wsPath = "/devtools/browser/abc123"
	const port = 55077
	wantWS := fmt.Sprintf("ws://127.0.0.1:%d%s", port, wsPath)

	l := &Launcher{
		BinaryPath: publishingChromium(t, marker, profile, port, wsPath),
		Runner:     shortBootRunner(5 * time.Second),
		Hostname:   testHost,
	}
	// DebuggingPort ZERO: the endpoint comes from the profile, which is the
	// whole point of decision 75. No port is chosen, so none can collide.
	b, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: profile, DebuggingPort: 0,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	reapBrowser(t, b)

	if b.WebSocketURL() != wantWS {
		t.Errorf("WebSocketURL = %q, want %q", b.WebSocketURL(), wantWS)
	}
	if b.PID() <= 0 {
		t.Error("no pid")
	}
	if !awaitMarker(t, marker) {
		t.Error("the browser binary never ran, yet Launch succeeded")
	}
}

// The escalation in SignalStop reaches the process GROUP, and only because the
// launch set Setpgid. It is set at the one place that starts a browser so no
// launch can be written without it — this asserts that place kept doing it.
func TestLaunchStartsTheBrowserInItsOwnProcessGroup(t *testing.T) {
	profile := t.TempDir()

	l := &Launcher{
		BinaryPath: publishingChromium(t, filepath.Join(t.TempDir(), "started"),
			profile, 55078, "/devtools/browser/x"),
		Runner:   shortBootRunner(5 * time.Second),
		Hostname: testHost,
	}
	b, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: profile, DebuggingPort: 0,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	reapBrowser(t, b)

	if b.cmd.SysProcAttr == nil || !b.cmd.SysProcAttr.Setpgid {
		t.Fatal("the browser was not started in its own process group; SignalStop's " +
			"escalation would signal this process's group instead of the renderers")
	}
}

// The order that matters most: a profile that cannot be proven stale must stop
// the launch BEFORE a process exists. Reclaiming after the start would delete
// the lock of the browser just started, and starting anyway would put two
// browsers on one profile.
func TestLaunchRefusesBeforeStartingWhenTheProfileIsHeld(t *testing.T) {
	profile := t.TempDir()
	marker := filepath.Join(t.TempDir(), "started")
	// A lock naming a live pid on this host: this test's own process.
	writeSingletons(t, profile, fmt.Sprintf("%s-%d", testHost, os.Getpid()))

	l := &Launcher{
		BinaryPath: fakeChromium(t, marker),
		Runner:     shortBootRunner(5 * time.Second),
		Hostname:   testHost,
	}
	_, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: profile, DebuggingPort: 9222,
	})
	if !errors.Is(err, ErrProfileHeldByLiveBrowser) {
		t.Fatalf("got %v, want ErrProfileHeldByLiveBrowser", err)
	}
	if awaitMarkerFor(t, marker, 500*time.Millisecond) {
		t.Fatal("a browser was started despite the profile being held — the reclaim " +
			"ran after the launch, or not at all")
	}
	if left := remaining(t, profile); len(left) != 3 {
		t.Fatalf("the held profile was reclaimed anyway: %v remain", left)
	}
}

// A browser that never answers must not be left running. An abandoned Chromium
// keeps the profile lock, and the NEXT launch would then fail for a reason
// that has nothing to do with why this one did.
func TestLaunchStopsTheBrowserWhenTheEndpointNeverAnswers(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	l := &Launcher{
		BinaryPath: fakeChromium(t, marker),
		// TWO SECONDS, NOT 300ms, AND THE REASON IS A RACE THIS TEST HAD WITH
		// ITSELF. Launch kills the browser when the endpoint does not answer —
		// that is the behaviour under test — but the fake writes its marker
		// from a shell that has to be scheduled first. With a 300ms budget, a
		// loaded host kills the shell before it touches the file, and the test
		// fails on its own PRECONDITION ("the browser never started"), not on
		// the behaviour.
		//
		// Measured 2026-08-20: 0 of 6 failures running alone, 2 of 2 under the
		// whole-repo coverage run, where instrumenting every package makes
		// process start-up slow enough to lose the race every time.
		//
		// Nothing is weakened by the larger budget: the endpoint still never
		// answers, Launch still fails, and the cleanup is still what is
		// asserted. The sibling test below already uses 5s for the same reason.
		Runner:   shortBootRunner(2 * time.Second),
		Hostname: testHost,
	}

	// A port nothing serves: the endpoint never answers.
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(free.Addr().String())
	port, _ := strconv.Atoi(portStr)
	_ = free.Close()

	_, err = l.Launch(context.Background(), LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: port,
	})
	if err == nil {
		t.Fatal("Launch succeeded with no endpoint")
	}
	if !awaitMarker(t, marker) {
		t.Fatal("the browser never started, so this test did not exercise the cleanup")
	}
	// The error must name how it was stopped: a stop whose form nobody records
	// is the trap that cost the study a comparison run.
	if !strings.Contains(err.Error(), "stopped_via=") {
		t.Errorf("the failure does not say how the browser was stopped: %v", err)
	}
}

// The file exists before it is COMPLETE: Chromium writes it in one go, but a
// reader can arrive mid-write and see only the port line. Accepting that would
// hand back a browser with no ws path, and every later CDP call would fail for
// a reason that looks unrelated.
func TestLaunchWaitsThroughAHalfWrittenEndpointFile(t *testing.T) {
	profile := t.TempDir()
	const wsPath = "/devtools/browser/late"
	const port = 55079
	wantWS := fmt.Sprintf("ws://127.0.0.1:%d%s", port, wsPath)

	l := &Launcher{
		BinaryPath: halfWrittenThenCompleteChromium(t, profile, port, wsPath),
		Runner:     shortBootRunner(5 * time.Second),
		Hostname:   testHost,
	}
	b, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: profile, DebuggingPort: 0,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	reapBrowser(t, b)

	if b.WebSocketURL() != wantWS {
		t.Fatalf("WebSocketURL = %q, want %q — the half-written file was accepted",
			b.WebSocketURL(), wantWS)
	}
}

// The boot budget has to bind, and it has to come from the policy. A launcher
// with its own private timeout would be a budget nobody can see.
func TestLaunchIsBoundedByTheBootBudget(t *testing.T) {
	l := &Launcher{
		BinaryPath: fakeChromium(t, filepath.Join(t.TempDir(), "started")),
		Runner:     shortBootRunner(250 * time.Millisecond),
		Hostname:   testHost,
	}
	free, _ := net.Listen("tcp", "127.0.0.1:0")
	_, portStr, _ := net.SplitHostPort(free.Addr().String())
	port, _ := strconv.Atoi(portStr)
	_ = free.Close()

	start := time.Now()
	_, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: port,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Launch succeeded with no endpoint")
	}
	// Budget plus one shutdown; anything near the 30s default means the boot
	// class was ignored.
	if elapsed > 5*time.Second {
		t.Fatalf("Launch took %v on a 250ms boot budget", elapsed)
	}
}

func TestLaunchRequiresABinary(t *testing.T) {
	l := &Launcher{Runner: shortBootRunner(time.Second)}
	if _, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: 9222,
	}); err == nil {
		t.Fatal("Launch with no binary path succeeded")
	}
}

// A leftover DevToolsActivePort is the one way reading the profile could
// reproduce the failure it removes. Chromium deletes the file on a clean exit,
// but a crashed browser leaves it behind — and a stale file points at a port
// that is now free, or that another browser has since taken. That second case
// is precisely "drive someone else's browser", which in this stack means
// someone else's WhatsApp account.
func TestLaunchIgnoresAStaleEndpointFileFromAPreviousRun(t *testing.T) {
	profile := t.TempDir()
	const stalePort = 40001
	const wsPath = "/devtools/browser/fresh"
	const freshPort = 55080

	// The corpse of a previous run, pointing somewhere else entirely.
	stale := fmt.Sprintf("%d\n/devtools/browser/STALE\n", stalePort)
	if err := os.WriteFile(filepath.Join(profile, activePortFile), []byte(stale), 0o600); err != nil {
		t.Fatalf("write stale endpoint file: %v", err)
	}

	l := &Launcher{
		BinaryPath: publishingChromium(t, filepath.Join(t.TempDir(), "started"),
			profile, freshPort, wsPath),
		Runner:   shortBootRunner(5 * time.Second),
		Hostname: testHost,
	}
	b, err := l.Launch(context.Background(), LaunchConfig{ProfileDir: profile})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	reapBrowser(t, b)

	want := fmt.Sprintf("ws://127.0.0.1:%d%s", freshPort, wsPath)
	if got := b.WebSocketURL(); got != want {
		t.Fatalf("WebSocketURL = %q, want %q — the stale file was believed", got, want)
	}
}

// The pinned-port route, which exists because Chromium leaves us nothing else.
//
// MEASURED (F102): with an explicit --remote-debugging-port, Chromium does NOT
// write DevToolsActivePort at all — it only answers HTTP on the number it was
// given. So an override cannot be served from the profile, and this test keeps
// the other route honest rather than asserting a preference.
//
// The double is a real HTTP server answering /json/version the way Chromium
// does, for the same reason the ephemeral double writes a real two-line file:
// a double that invents its own shape proves only that the reader reads it.
func TestLaunchUsesTheHTTPEndpointWhenThePortIsPinned(t *testing.T) {
	profile := t.TempDir()
	const wantWS = "ws://127.0.0.1:9222/devtools/browser/pinned"
	port := devToolsServer(t, func() string { return wantWS })

	l := &Launcher{
		BinaryPath: fakeChromium(t, filepath.Join(t.TempDir(), "started")),
		Runner:     shortBootRunner(5 * time.Second),
		Hostname:   testHost,
	}
	b, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: profile, DebuggingPort: port,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	reapBrowser(t, b)

	if b.WebSocketURL() != wantWS {
		t.Fatalf("WebSocketURL = %q, want %q", b.WebSocketURL(), wantWS)
	}
	// And nothing was invented in the profile: the pinned route must not depend
	// on a file Chromium never writes.
	if _, err := os.Stat(filepath.Join(profile, activePortFile)); err == nil {
		t.Fatal("the pinned route wrote an endpoint file; real Chromium does not")
	}
}
