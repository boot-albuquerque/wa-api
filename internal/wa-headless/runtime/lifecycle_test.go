package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/liveness"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/events"
	"wa-api/internal/wa-headless/spa"
)

type busRecorder struct {
	mu  sync.Mutex
	got []events.Event
}

func (b *busRecorder) observe(e events.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.got = append(b.got, e)
}

func (b *busRecorder) snapshot() []events.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]events.Event{}, b.got...)
}

func (b *busRecorder) types() []events.Type {
	out := []events.Type{}
	for _, e := range b.snapshot() {
		out = append(out, e.Type)
	}
	return out
}

// The wiring's whole purpose, end to end: a Holder booting a real browser puts
// session.ready on the bus, and stopping it puts session.stopped there.
//
// It is deliberately not a unit test of the adapter function. The adapter is
// three lines; what could break is the WIRING — an observer set after the
// config was copied, a hub attached to a Holder that already booted — and only
// a real boot exercises that.
func TestAttachHub_ReadyAndStoppedReachTheBus(t *testing.T) {
	hub := events.NewHub()
	rec := &busRecorder{}
	stop := hub.Subscribe(rec.observe, events.SessionReady, events.SessionStopped)
	defer stop()

	h := NewHolder(holderConfig(t, t.TempDir()))
	if !h.AttachHub(hub) {
		t.Fatal("AttachHub refused a fresh Holder")
	}
	if _, err := h.Session(context.Background()); err != nil {
		t.Fatalf("Session: %v", err)
	}
	if got := rec.types(); len(got) != 1 || got[0] != events.SessionReady {
		t.Fatalf("after boot the bus has %v, want [%s]", got, events.SessionReady)
	}
	if r := rec.snapshot()[0].Reason; r != reasonFresh {
		t.Errorf("ready reason %q, want %q", r, reasonFresh)
	}

	h.Stop(context.Background())
	got := rec.snapshot()
	if len(got) != 2 || got[1].Type != events.SessionStopped {
		t.Fatalf("after Stop the bus has %v, want a %s", rec.types(), events.SessionStopped)
	}
	if got[1].Reason == "" {
		t.Error("the stop carries no StopVia; a graceful close and a kill look the same")
	}
	for _, e := range got {
		if e.Origin != events.SourceLocal {
			t.Errorf("%s came from %q, want %q", e.Type, e.Origin, events.SourceLocal)
		}
	}
}

// A hub attached to a Holder that has ALREADY booted would miss the one fact it
// most wants. Refusing is louder than a silent no-op and quieter than a panic
// in a wiring path — and this is the test that keeps it from becoming one.
func TestAttachHub_RefusesAfterTheBootItWouldHaveMissed(t *testing.T) {
	h := NewHolder(holderConfig(t, t.TempDir()))
	defer h.Stop(context.Background())
	if _, err := h.Session(context.Background()); err != nil {
		t.Fatalf("Session: %v", err)
	}
	if h.AttachHub(events.NewHub()) {
		t.Fatal("AttachHub accepted a Holder that had already booted; the hub would be deaf to its ready")
	}
}

// A failed boot is a lifecycle fact too, and it carries the STAGE. A caller
// told only "the session is not up" cannot tell a page still mounting from a
// QR screen that needs a human.
func TestAttachHub_ABootThatFailsSaysWhichStage(t *testing.T) {
	hub := events.NewHub()
	rec := &busRecorder{}
	defer hub.Subscribe(rec.observe, events.SessionBootFailed)()

	cfg := holderConfig(t, t.TempDir())
	cfg.NavigateURL = pageServer(t, `<html><body>nothing here</body></html>`)
	cfg.SettleBudget = 2 * time.Second
	cfg.RequiredModules = []spa.Module{}

	h := NewHolder(cfg)
	defer h.Stop(context.Background())
	if !h.AttachHub(hub) {
		t.Fatal("AttachHub refused a fresh Holder")
	}
	if _, err := h.Session(context.Background()); err == nil {
		t.Fatal("a blank page must not boot")
	}
	got := rec.snapshot()
	if len(got) != 1 || got[0].Type != events.SessionBootFailed {
		t.Fatalf("the bus has %v, want one %s", rec.types(), events.SessionBootFailed)
	}
	if got[0].Reason != string(core.StageNotReady) {
		t.Fatalf("reason %q, want %q", got[0].Reason, core.StageNotReady)
	}
	// E DIZ CONTRA O QUE MORREU, nao so' onde parou.
	//
	// O estagio sozinho nao distingue um boot que morre contra uma tela de QR de
	// um que morre contra pagina que nao responde — os dois chegam a not_ready,
	// por motivos opostos, e o primeiro e' o que o upstream chama de falha de
	// AUTENTICACAO. Ate' este campo existir, a classe vivia so' na mensagem de
	// erro, e mensagem de erro nao entra neste barramento.
	//
	// A asserção e' aqui, e nao no pacote events, porque o elo que ela protege e'
	// o repasse do fato ate' o hub: um controle negativo que apagou esse repasse
	// COMPILOU e nao foi pego por nenhum teste unitario.
	if got[0].PageClass == "" {
		t.Fatal("the boot failure reached the bus with no page class, so a " +
			"subscriber cannot tell an unauthenticated page from a broken one")
	}
	if got[0].PageClass == string(spa.ClassAppReady) {
		t.Fatalf("a failed boot reported the ready class %q", got[0].PageClass)
	}
	t.Logf("blank page classified as %q", got[0].PageClass)
}

// THE FIRST VERDICT IS AN EVENT and a repeat is not.
//
// The watcher is driven with a fake process signal rather than a real browser,
// because what is under test is the TRANSITION RULE, and a real process obliges
// the test to kill something to make the state move — which measures the kill.
func TestStateWatcher_OnlyTransitionsReachTheBus(t *testing.T) {
	hub := events.NewHub()
	rec := &busRecorder{}
	defer hub.Subscribe(rec.observe, events.SessionStateChanged)()

	// A process that is gone short-circuits Check before any page probe, so
	// this drives the signal without a browser and without a fake page.
	//
	// IT STARTS GONE, and that is not laziness. Starting ALIVE with an
	// evaluator that fails is not one state: the monitor reports PAGE_SLOW for
	// the first failures and PAGE_UNRESPONSIVE once the streak crosses its
	// threshold, so the "unchanging state" leg would be measuring a state that
	// changes by itself. The first draft of this test did exactly that and
	// accused the watcher of repeating itself.
	alive := false
	var amu sync.Mutex
	checker := liveness.New(func() bool {
		amu.Lock()
		defer amu.Unlock()
		return alive
	}, engine.NewRunner(), func(ctx context.Context, expr string, out *string) error {
		// Never reached while alive is false; while true it must answer
		// something the monitor accepts as a live page.
		return errors.New("no page in this test")
	})

	w := NewStateWatcher(checker, hub, "test/state")
	w.interval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(ctx) }()

	// Several ticks in the SAME state must produce exactly one event.
	waitFor(t, func() bool { return len(rec.snapshot()) >= 1 })
	time.Sleep(80 * time.Millisecond)
	if n := len(rec.snapshot()); n != 1 {
		t.Fatalf("%d events for one unchanging state; a bus that repeats itself is a heartbeat", n)
	}
	first := rec.snapshot()[0]

	amu.Lock()
	alive = true
	amu.Unlock()
	waitFor(t, func() bool { return len(rec.snapshot()) >= 2 })
	cancel()
	<-done

	got := rec.snapshot()
	if got[1].State == first.State {
		t.Fatalf("the second event repeats %q; only a transition is an event", first.State)
	}
	if first.State != string(liveness.SignalProcessGone) {
		t.Fatalf("first state = %q, want %q", first.State, liveness.SignalProcessGone)
	}
	if got[1].State != string(liveness.SignalPageSlow) {
		t.Fatalf("state after the process came back = %q, want %q", got[1].State, liveness.SignalPageSlow)
	}
	if got[1].Origin != events.SourceLocal {
		t.Errorf("origin = %q, want %q", got[1].Origin, events.SourceLocal)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never held within 5s")
}
