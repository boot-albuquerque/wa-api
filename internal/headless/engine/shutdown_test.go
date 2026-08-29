package engine

import (
	"context"
	"sync"
	"testing"
	"time"
)

// deadEndpoint is an address nothing listens on, so the CDP command fails for a
// reason that is real (no browser there) rather than stubbed.
const deadEndpoint = "ws://127.0.0.1:1/devtools/browser/none"

// fakeProcess records the ORDER of what CleanStop did to it. Order is the point:
// a shutdown that signals before waiting gives every stop the dirty path
// underneath while still reporting the clean label.
type fakeProcess struct {
	profileDir string
	mu         sync.Mutex
	calls      []string
	url        string
	exits      bool
	signaled   chan struct{}
}

func newFakeProcess(url string, exits bool) *fakeProcess {
	return &fakeProcess{url: url, exits: exits, signaled: make(chan struct{}, 1)}
}

// newFakeProcessInProfile is for tests that read the suspect marker back, so
// the directory comes from the caller rather than being invented here.
func newFakeProcessInProfile(url string, exits bool, profileDir string) *fakeProcess {
	p := newFakeProcess(url, exits)
	p.profileDir = profileDir
	return p
}

func (f *fakeProcess) record(name string) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
}

func (f *fakeProcess) sequence() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeProcess) WebSocketURL() string {
	f.record("close")
	return f.url
}

func (f *fakeProcess) WaitExit(ctx context.Context) error {
	f.record("wait")
	if f.exits {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

// ProfileDir is where CleanStop writes the suspect marker after a dirty stop.
//
// Tests that care about the marker pass a REAL temporary directory. An empty
// one makes MarkSessionSuspect return early, so every dirty-path test would
// pass without the marking ever running — the "double more permissive than
// production" trap, hiding the whole mechanism.
func (f *fakeProcess) ProfileDir() string { return f.profileDir }

func (f *fakeProcess) SignalStop(ctx context.Context) error {
	f.record("signal")
	select {
	case f.signaled <- struct{}{}:
	default:
	}
	return nil
}

// shortShutdownRunner keeps the fallback path from costing the real budget.
func shortShutdownRunner() *Runner {
	p := DefaultDeadlines
	p.Shutdown = 150 * time.Millisecond
	return &Runner{Policy: p, Log: NewRunner().Log}
}

func TestCleanStopTakesTheProtocolPath(t *testing.T) {
	f := startFakeBrowser(t, echoOK)
	p := newFakeProcess(f.wsURL(), true)

	via := CleanStop(context.Background(), shortShutdownRunner(), p)

	if via != StopViaBrowserClose {
		t.Fatalf("stopped via %q, want %q", via, StopViaBrowserClose)
	}
	if !via.Clean() {
		t.Error("the protocol path must classify as clean")
	}
	for _, c := range p.sequence() {
		if c == "signal" {
			t.Fatal("a signal was sent on the clean path; SIGTERM corrupts session state")
		}
	}
}

// The sequence IS the contract. Inverting the two calls passes every outcome
// assertion above and still reintroduces the defect.
func TestCleanStopWaitsBeforeItEverSignals(t *testing.T) {
	f := startFakeBrowser(t, echoOK)
	p := newFakeProcess(f.wsURL(), false) // asked politely, refuses to leave

	via := CleanStop(context.Background(), shortShutdownRunner(), p)

	if via != StopViaDirtySignalExitTimeout {
		t.Fatalf("stopped via %q, want %q", via, StopViaDirtySignalExitTimeout)
	}
	if via.Clean() {
		t.Error("a signal-based stop must never classify as clean")
	}

	seq := p.sequence()
	waitAt, signalAt := firstIndexOf(seq, "wait"), firstIndexOf(seq, "signal")
	if waitAt < 0 || signalAt < 0 {
		t.Fatalf("sequence %v: expected both a wait and a signal", seq)
	}
	if signalAt < waitAt {
		t.Fatalf("sequence %v: the signal was sent BEFORE waiting for the exit — "+
			"every stop would carry the dirty shutdown underneath", seq)
	}
}

// The deliberate divergence from the study: a lost acknowledgement is not a
// refusal. The browser was measured replying in 2 ms and then dropping the
// socket, so signalling on a missing reply would corrupt the session the clean
// path just saved.
func TestCleanStopTrustsTheExitNotTheAcknowledgement(t *testing.T) {
	p := newFakeProcess(deadEndpoint, true) // the command cannot be delivered

	via := CleanStop(context.Background(), shortShutdownRunner(), p)

	if via != StopViaBrowserCloseUnconfirmed {
		t.Fatalf("stopped via %q, want %q", via, StopViaBrowserCloseUnconfirmed)
	}
	if !via.Clean() {
		t.Error("a process that left on its own within budget stopped cleanly")
	}
	for _, c := range p.sequence() {
		if c == "signal" {
			t.Fatal("a signal was sent to a process that had already left")
		}
	}
}

func TestCleanStopLabelsARefusalDistinctly(t *testing.T) {
	p := newFakeProcess(deadEndpoint, false)

	via := CleanStop(context.Background(), shortShutdownRunner(), p)

	if via != StopViaDirtySignalCloseRefused {
		t.Fatalf("stopped via %q, want %q", via, StopViaDirtySignalCloseRefused)
	}
	select {
	case <-p.signaled:
	default:
		t.Fatal("the process neither answered nor left, and no signal was sent — " +
			"an orphan browser holds the profile lock and blocks the next boot")
	}
}

func TestCleanStopOnNothingIsNoop(t *testing.T) {
	if via := CleanStop(context.Background(), shortShutdownRunner(), nil); via != StopViaNoop {
		t.Fatalf("got %q, want %q", via, StopViaNoop)
	}
}

// A stopped_via that nobody records is the trap that cost the study a run. The
// shutdown must leave its own trace.
func TestCleanStopIsRecorded(t *testing.T) {
	f := startFakeBrowser(t, echoOK)
	r := shortShutdownRunner()

	CleanStop(context.Background(), r, newFakeProcess(f.wsURL(), true))

	var sawClose, sawWait bool
	for _, rec := range r.Log.Records() {
		if rec.Op != string(OpShutdown) {
			continue
		}
		switch rec.Label {
		case opLabelClose:
			sawClose = true
		case opLabelWaitExit:
			sawWait = true
		}
	}
	if !sawClose || !sawWait {
		t.Fatalf("shutdown left no trace (close=%v wait=%v)", sawClose, sawWait)
	}
}

// The stop must stay bounded even when nothing cooperates: it blocks its
// caller, and an unbounded block inside a limited slot is the failure mode the
// root CLAUDE.md names as this project's standing invariant.
func TestCleanStopIsBounded(t *testing.T) {
	f := startFakeBrowser(t, func(cmd cdpMessage) (cdpMessage, action) {
		return cdpMessage{}, actionSilent // accepted, never answered
	})
	r := shortShutdownRunner()
	p := newFakeProcess(f.wsURL(), false)

	start := time.Now()
	via := CleanStop(context.Background(), r, p)
	elapsed := time.Since(start)

	if via.Clean() {
		t.Errorf("got %q; nothing cooperated, so nothing was clean", via)
	}
	// Three shutdown budgets is generous; the point is that it terminates on a
	// budget rather than on the peer's goodwill.
	if max := 3 * r.Policy.Shutdown; elapsed > max {
		t.Fatalf("CleanStop took %v with an uncooperative peer, over the %v bound", elapsed, max)
	}
}

// firstIndexOf returns where name first appears, or -1.
func firstIndexOf(seq []string, name string) int {
	for i, c := range seq {
		if c == name {
			return i
		}
	}
	return -1
}
