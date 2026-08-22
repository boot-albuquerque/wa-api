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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (

	// versionEndpoint is where Chromium publishes its browser-level CDP URL.
	// It is the ONLY way in when the caller pinned a port — see awaitEndpoint.
	versionEndpoint = "/json/version"

	// activePortFile is the file Chromium writes inside the user-data-dir with
	// the port it actually bound and the browser ws path. It is the identity
	// the launcher trusts (F102, decision 75).
	activePortFile = "DevToolsActivePort"
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
	// HTTPClient talks to the DevTools endpoint on the pinned-port path.
	// Nil uses a default.
	HTTPClient *http.Client

	// Hostname identifies this host to the profile reclaim. Empty detects it.
	Hostname string
	// AllowForeignProfile permits reclaiming a profile locked by another host.
	// Set it only where single ownership is proven by a lease, never by a guess.
	AllowForeignProfile bool
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

	// A STALE DevToolsActivePort is the one way this scheme could reproduce the
	// very failure it removes: Chromium deletes the file on a clean exit, but a
	// crashed browser leaves it behind, and reading it would point at a port
	// that is now free — or worse, at one another browser has since taken.
	// Removing it BEFORE the process starts makes staleness impossible: what we
	// read afterwards can only have been written by the process we launched.
	if err := removeActivePortFile(cfg.ProfileDir); err != nil {
		return nil, fmt.Errorf("launch: %w", err)
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

	browser := newBrowser(cmd, "", cfg.ProfileDir)

	wsURL, err := l.awaitEndpoint(ctx, runner, cfg)
	if err != nil {
		via := CleanStop(context.Background(), runner, browser)
		return nil, fmt.Errorf("launch: no endpoint for the profile we launched "+
			"(stopped_via=%s): %w", via, err)
	}
	browser.wsURL = wsURL
	return browser, nil
}

// awaitEndpoint returns the browser WebSocket URL, by whichever route the
// caller's port choice leaves open. The two routes are not a preference: which
// one exists is decided by Chromium, and it was MEASURED (F102, decision 75).
//
//	DebuggingPort == 0  Chromium binds an ephemeral port and PUBLISHES it in
//	                    <ProfileDir>/DevToolsActivePort. We read the profile.
//	DebuggingPort  > 0  Chromium does NOT write that file at all. The endpoint
//	                    exists only over HTTP, on the number the caller chose.
//
// The asymmetry is the reason to prefer port 0, and the reason the registry
// will. Reading the profile ties the endpoint to the browser WE launched;
// polling a port accepts whatever answers on it, and with N sessions racing for
// numbers that means one session driving another's browser — in this stack,
// another WhatsApp account, silently.
//
// Pinning a port is therefore the caller taking that risk explicitly, which is
// exactly what an override should be.
func (l *Launcher) awaitEndpoint(ctx context.Context, runner *Runner, cfg LaunchConfig) (string, error) {
	var wsURL string
	err := runner.Do(ctx, OpBoot, "launch/await-endpoint", func(ctx context.Context) error {
		for {
			// The "not yet" cases are decided in ONE place per route. A second
			// check here would look like defence and act like camouflage: with
			// it duplicated, deleting either one changes nothing, so no test
			// can hold either one.
			u, err := l.endpointOnce(ctx, cfg)
			if err == nil {
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

// endpointOnce is a single attempt down whichever route applies.
func (l *Launcher) endpointOnce(ctx context.Context, cfg LaunchConfig) (string, error) {
	if cfg.DebuggingPort > 0 {
		return l.readEndpoint(ctx, fmt.Sprintf("http://%s:%d%s",
			localEndpointHost, cfg.DebuggingPort, versionEndpoint))
	}
	port, path, err := readActivePort(cfg.ProfileDir)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ws://%s:%d%s", localEndpointHost, port, path), nil
}

// readEndpoint asks the DevTools endpoint over HTTP. It is the pinned-port
// route ONLY: with an explicit --remote-debugging-port Chromium does not write
// DevToolsActivePort, so there is nothing in the profile to read.
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

// activePortPath is where Chromium publishes the endpoint of the browser that
// owns this profile.
func activePortPath(profileDir string) string {
	return filepath.Join(profileDir, activePortFile)
}

// removeActivePortFile drops a leftover endpoint file. A missing file is the
// expected case and not an error; anything else is, because failing to remove
// it would leave us reading a stale endpoint.
func removeActivePortFile(profileDir string) error {
	if err := os.Remove(activePortPath(profileDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale %s: %w", activePortFile, err)
	}
	return nil
}

// readActivePort parses the two lines Chromium writes: the bound port and the
// browser ws path.
//
// A partially written file is a "not yet", not a failure: the browser writes it
// in one go, but a reader can still arrive mid-write, and treating that as a
// hard error would turn a normal race into a boot failure.
func readActivePort(profileDir string) (int, string, error) {
	raw, err := os.ReadFile(activePortPath(profileDir))
	if err != nil {
		return 0, "", err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		return 0, "", fmt.Errorf("%s has %d line(s), want 2", activePortFile, len(lines))
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, "", fmt.Errorf("%s: port %q: %w", activePortFile, lines[0], err)
	}
	if port <= 0 {
		return 0, "", fmt.Errorf("%s: port %d is not usable", activePortFile, port)
	}
	path := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(path, "/") {
		return 0, "", fmt.Errorf("%s: ws path %q does not start with /", activePortFile, path)
	}
	return port, path, nil
}
