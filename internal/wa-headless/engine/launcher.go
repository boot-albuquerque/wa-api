package engine

// Launching a browser: reclaim the profile, start the process, wait for it to
// answer on its DevTools endpoint.
//
// The ORDER is the contract, and it runs the opposite way from the shutdown:
//
//	1. reclaim the profile — and REFUSE to launch if it cannot be proven stale;
//	2. only then start the process;
//	3. wait for /json/version under a budget, and clean up if it never answers.
//
// Step 1 comes first because Chromium will not boot on a profile that still
// carries its Singleton files, and because a reclaim after the launch would be
// deleting the lock of the browser we just started. Step 3 cleans up because a
// half-started Chromium holds the profile lock and blocks every later boot —
// the orphan problem, arriving through the door marked "startup failed".
//
// Study origin: scripts/chromium-study/p3_launcher.go, launchOne.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"syscall"
	"time"
)

const (
	// versionEndpoint is where Chromium publishes its browser-level CDP URL.
	versionEndpoint = "/json/version"
	// bootPollInterval is how often the endpoint is asked. The clock is on THIS
	// side — a browser that never comes up cannot be relied on to time itself
	// out, which is the same lesson as the page-side poll of ARMADILHAS 19.
	bootPollInterval = 50 * time.Millisecond
	// localEndpointHost is what the launcher dials. The process is local
	// whatever address it bound, and dialling the bind address would fail
	// outright for 0.0.0.0.
	localEndpointHost = "127.0.0.1"
)

// Launcher starts browsers under this package's policies.
type Launcher struct {
	// BinaryPath is the Chromium executable.
	BinaryPath string
	// Runner supplies the budgets and records what happened.
	Runner *Runner
	// Hostname identifies this host to the profile reclaim. Empty detects it.
	Hostname string
	// AllowForeignProfile permits reclaiming a profile locked by another host.
	// Set it only where single ownership is proven by a lease, never by a guess.
	AllowForeignProfile bool
	// HTTPClient talks to the DevTools endpoint. Nil uses a default.
	HTTPClient *http.Client
}

// Launch brings up one browser and returns it once its endpoint answers.
//
// On any failure after the process started, the process is stopped through
// CleanStop before the error is returned: an abandoned Chromium keeps the
// profile lock, and the next launch would then fail for a reason that has
// nothing to do with why this one did.
func (l *Launcher) Launch(ctx context.Context, cfg LaunchConfig) (*Browser, error) {
	if l.BinaryPath == "" {
		return nil, fmt.Errorf("launch: BinaryPath is empty")
	}
	runner := l.Runner
	if runner == nil {
		runner = NewRunner()
	}

	if _, err := ReclaimProfile(cfg.ProfileDir, ReclaimOptions{
		Hostname:         l.Hostname,
		AllowForeignHost: l.AllowForeignProfile,
		ProcessAlive:     ProcessAlive,
	}); err != nil {
		return nil, fmt.Errorf("launch: %w", err)
	}

	flags, err := BuildFlags(cfg)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(l.BinaryPath, flags...)
	// Setpgid is what makes SignalStop's escalation reach the renderers. It is
	// set HERE, at the only place that starts a browser, so no launch can be
	// written without it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Deliberately NOT exec.CommandContext: that kills the process when ctx
	// ends, which would be a signal-based stop arriving from outside the
	// shutdown path — invariant 3, and the one the static gate cannot see
	// because the kill happens inside os/exec.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("launch: start %s: %w", l.BinaryPath, err)
	}

	browser := newBrowser(cmd, "")

	wsURL, err := l.awaitEndpoint(ctx, runner, cfg.DebuggingPort)
	if err != nil {
		via := CleanStop(context.Background(), runner, browser)
		return nil, fmt.Errorf("launch: browser never answered on %s (stopped_via=%s): %w",
			versionEndpoint, via, err)
	}
	browser.wsURL = wsURL
	return browser, nil
}

// awaitEndpoint polls /json/version until it yields a WebSocket URL.
func (l *Launcher) awaitEndpoint(ctx context.Context, runner *Runner, port int) (string, error) {
	url := fmt.Sprintf("http://%s:%d%s", localEndpointHost, port, versionEndpoint)

	var wsURL string
	err := runner.Do(ctx, OpBoot, "launch/await-endpoint", func(ctx context.Context) error {
		for {
			// The "not yet" cases are decided in readEndpoint, in ONE place.
			// A second `u != ""` here would look like defence and act like
			// camouflage: with the check duplicated, deleting either one
			// changes nothing, so no test can hold either one.
			if u, err := l.readEndpoint(ctx, url); err == nil {
				wsURL = u
				return nil
			}
			select {
			case <-ctx.Done():
				// The class budget decides, not this loop. Returning the
				// context error lets Runner.Do classify it as the timeout it is.
				return ctx.Err()
			case <-time.After(bootPollInterval):
			}
		}
	})
	if err != nil {
		return "", err
	}
	return wsURL, nil
}

func (l *Launcher) readEndpoint(ctx context.Context, url string) (string, error) {
	client := l.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: status %d", versionEndpoint, resp.StatusCode)
	}

	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.WebSocketDebuggerURL == "" {
		// The endpoint answers before the browser target is ready. An empty URL
		// is "not yet", not "never" — treating it as success would hand the
		// caller a browser it cannot address, and every later CDP call would
		// fail for a reason that looks unrelated.
		return "", fmt.Errorf("%s: no webSocketDebuggerUrl yet", versionEndpoint)
	}
	return payload.WebSocketDebuggerURL, nil
}
