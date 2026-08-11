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

// devToolsServer answers /json/version the way Chromium does.
func devToolsServer(t *testing.T, wsURL func() string) (port int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != versionEndpoint {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"Browser":"HeadlessChrome/151.0.0.0","webSocketDebuggerUrl":%q}`, wsURL())
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
	const wantWS = "ws://127.0.0.1:9222/devtools/browser/abc123"
	port := devToolsServer(t, func() string { return wantWS })

	l := &Launcher{
		BinaryPath: fakeChromium(t, marker),
		Runner:     shortBootRunner(5 * time.Second),
		Hostname:   testHost,
	}
	b, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: port,
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
	port := devToolsServer(t, func() string { return "ws://127.0.0.1:1/devtools/browser/x" })

	l := &Launcher{
		BinaryPath: fakeChromium(t, filepath.Join(t.TempDir(), "started")),
		Runner:     shortBootRunner(5 * time.Second),
		Hostname:   testHost,
	}
	b, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: port,
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
		Runner:     shortBootRunner(300 * time.Millisecond),
		Hostname:   testHost,
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

// The endpoint answers before the browser target exists, with an empty URL.
// Accepting it would hand back a browser that cannot be addressed, and every
// later CDP call would fail for a reason that looks unrelated.
func TestLaunchWaitsThroughAnEmptyWebSocketURL(t *testing.T) {
	var ready bool
	const wantWS = "ws://127.0.0.1:9222/devtools/browser/late"
	port := devToolsServer(t, func() string {
		if !ready {
			ready = true
			return "" // the first answer is "not yet"
		}
		return wantWS
	})

	l := &Launcher{
		BinaryPath: fakeChromium(t, filepath.Join(t.TempDir(), "started")),
		Runner:     shortBootRunner(5 * time.Second),
		Hostname:   testHost,
	}
	b, err := l.Launch(context.Background(), LaunchConfig{
		ProfileDir: t.TempDir(), DebuggingPort: port,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	reapBrowser(t, b)

	if b.WebSocketURL() != wantWS {
		t.Fatalf("WebSocketURL = %q, want %q — the empty first answer was accepted",
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
