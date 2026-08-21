package events

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// fakePage answers the pump's two scripts. It is scripted per cycle so a test
// can say "install fresh, then deliver these, then reload".
type fakePage struct {
	mu sync.Mutex
	// installs counts calls; installAlready decides whether each answers
	// "already" — false means a FRESH install, which is what a reload looks
	// like from the pump's side.
	installs      int
	freshAt       map[int]bool
	notReadyUntil int

	rows    [][]string
	dropped []int64
	seen    []int64
	drains  int

	uninstalled bool
}

func (f *fakePage) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.Contains(expr, "NO_COLLECTIONS"): // the install script
		f.installs++
		if f.installs <= f.notReadyUntil {
			*out = `{"installed":false,"reason":"NO_COLLECTIONS"}`
			return nil
		}
		fresh := f.freshAt[f.installs]
		*out = fmt.Sprintf(`{"installed":true,"already":%t}`, !fresh)
		return nil
	case strings.Contains(expr, "s.buf = [];") && strings.Contains(expr, "rows: rows"):
		i := f.drains
		f.drains++
		rows := "[]"
		if i < len(f.rows) {
			rows = "[" + strings.Join(f.rows[i], ",") + "]"
		}
		// THE PAGE'S COUNTERS ARE MONOTONIC WITHIN AN INCARNATION, so past the
		// scripted cycles this repeats the LAST value instead of answering
		// zero. An earlier version answered zero and produced a failure that
		// blamed the accumulator: a double LESS faithful than production
		// invents defects as readily as a permissive one hides them.
		var d, s int64
		if len(f.dropped) > 0 {
			if i < len(f.dropped) {
				d = f.dropped[i]
			} else {
				d = f.dropped[len(f.dropped)-1]
			}
		}
		if len(f.seen) > 0 {
			if i < len(f.seen) {
				s = f.seen[i]
			} else {
				s = f.seen[len(f.seen)-1]
			}
		}
		*out = fmt.Sprintf(`{"ok":true,"rows":%s,"dropped":%d,"seen":%d}`, rows, d, s)
		return nil
	default: // uninstall
		f.uninstalled = true
		*out = `{"removed":true}`
		return nil
	}
}

func row(typ string, seq int64, msg string) string {
	return fmt.Sprintf(`{"type":%q,"seq":%d,"at":1787000000000,"chat":"1@c.us","msg":%q,"fromMe":true,"kind":"chat","ack":1,"bodyLen":7}`,
		typ, seq, msg)
}

func runPump(t *testing.T, f *fakePage, h *Hub, cycles int) {
	t.Helper()
	old := PollInterval
	PollInterval = time.Millisecond
	defer func() { PollInterval = old }()

	p := NewPump(engine.NewRunner(), f.eval, h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = p.Run(ctx) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		f.mu.Lock()
		d := f.drains
		f.mu.Unlock()
		// WAIT FOR THE CYCLE AFTER THE LAST ONE TO START. The drain counter
		// increments when a drain BEGINS, and the pump writes the stats when it
		// RETURNS — so stopping at exactly `cycles` cancelled the pump between
		// those two moments and the last drain's numbers were never applied.
		// Waiting for the next cycle to start proves the previous one finished.
		if d >= cycles+1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
}

// TestThePumpWaitsWhenThePageIsNotReadyYet. During boot and mid-reload the
// collections do not exist; failing there would make a normal moment look like
// a defect.
func TestThePumpWaitsWhenThePageIsNotReadyYet(t *testing.T) {
	f := &fakePage{notReadyUntil: 3, freshAt: map[int]bool{4: true},
		rows: [][]string{{row("message.added", 1, "A")}}}
	h := NewHub()
	var got int
	h.Subscribe(func(Event) { got++ })
	runPump(t, f, h, 1)
	if got != 1 {
		t.Fatalf("the pump gave up before the page was ready: delivered %d", got)
	}
	if s := h.Stats(); s.Reinstalls != 1 {
		t.Fatalf("a not-ready install was counted as a reinstall: %s", s)
	}
}

// TestTheFirstBatchAfterAnInstallIsReplay. After a reload the page refills its
// collections from history and the same 'add' fires for every message that ever
// arrived; a consumer counting those as new double-counts its whole history.
func TestTheFirstBatchAfterAnInstallIsReplay(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}, rows: [][]string{
		{row("message.added", 1, "OLD1"), row("message.added", 2, "OLD2")},
		{row("message.added", 3, "NEW")},
	}}
	h := NewHub()
	var replay, live []string
	h.Subscribe(func(e Event) {
		if e.Replay {
			replay = append(replay, e.MessageID)
		} else {
			live = append(live, e.MessageID)
		}
	})
	runPump(t, f, h, 2)
	if len(replay) != 2 {
		t.Fatalf("the first batch was not marked replay: %v", replay)
	}
	if len(live) != 1 || live[0] != "NEW" {
		t.Fatalf("the second batch is not live: %v", live)
	}
}

// TestTheCountersAccumulateAcrossAReload. The page's counters reset when the
// page does, and a stat that goes BACKWARDS is worse than no stat.
func TestTheCountersAccumulateAcrossAReload(t *testing.T) {
	f := &fakePage{
		freshAt: map[int]bool{1: true, 3: true}, // a reload before the third install
		rows: [][]string{
			{row("message.added", 1, "A")},
			{row("message.added", 2, "B")},
			{row("message.added", 1, "C")}, // the page's sequence restarts too
		},
		dropped: []int64{2, 5, 1},
		seen:    []int64{10, 20, 3},
	}
	h := NewHub()
	h.Subscribe(func(Event) {})
	runPump(t, f, h, 3)

	s := h.Stats()
	// 20 from the first incarnation, folded in at the reinstall, plus 3 from
	// the second. Overwriting would have left 3.
	if s.SeenInPage != 23 {
		t.Errorf("seenInPage = %d, want 23 (20 folded in + 3 since): %s", s.SeenInPage, s)
	}
	if s.DroppedInPage != 6 {
		t.Errorf("droppedInPage = %d, want 6 (5 folded in + 1 since): %s", s.DroppedInPage, s)
	}
	if s.Reinstalls != 2 {
		t.Errorf("reinstalls = %d, want 2: %s", s.Reinstalls, s)
	}
	// AND THE SEQUENCE RESTART MUST NOT LOOK LIKE A GAP. The page counts from
	// one again after a reload; carrying the old high-water mark would report
	// drops that never happened.
	if s.Gaps != 0 {
		t.Errorf("gaps = %d after a reload that restarted the sequence: %s", s.Gaps, s)
	}
}

// TestAnUnknownTypeIsDroppedRatherThanForwarded. The page and the Go file are
// versioned together; a name that does not match means one of them changed
// alone, and forwarding it would let a subscriber match on a meaningless string.
func TestAnUnknownTypeIsDroppedRatherThanForwarded(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}, rows: [][]string{{
		row("message.added", 1, "A"),
		row("something.invented", 2, "B"),
	}}}
	h := NewHub()
	var got []Type
	h.Subscribe(func(e Event) { got = append(got, e.Type) })
	runPump(t, f, h, 1)
	if len(got) != 1 || got[0] != MessageAdded {
		t.Fatalf("an unknown type was forwarded: %v", got)
	}
}

// TestUninstallIsCalledAndIsSafeWhenNothingIsInstalled.
func TestUninstallIsCalledAndIsSafeWhenNothingIsInstalled(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}}
	p := NewPump(engine.NewRunner(), f.eval, NewHub())
	if err := p.Uninstall(context.Background()); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.uninstalled {
		t.Fatal("Uninstall did not reach the page")
	}
}

// TestThePumpStopsWhenItsContextDoes. A pump that outlives its session keeps
// polling a page that is being torn down.
func TestThePumpStopsWhenItsContextDoes(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}}
	p := NewPump(engine.NewRunner(), f.eval, NewHub())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil on a cancelled context; the caller cannot tell it stopped")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the pump did not stop when its context was cancelled")
	}
}

// TestAnUnreadableAnswerIsAnErrorNotSilence.
func TestAnUnreadableAnswerIsAnErrorNotSilence(t *testing.T) {
	bad := func(_ context.Context, expr string, out *string) error {
		*out = "not json"
		return nil
	}
	p := NewPump(engine.NewRunner(), bad, NewHub())
	if err := p.Run(context.Background()); err == nil {
		t.Fatal("a page answering garbage was accepted")
	}
}
