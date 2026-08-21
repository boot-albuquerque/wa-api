package events

import (
	"context"
	"fmt"
	"os"
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

// pageNowMillis is the "at" the fake page stamps. The message timestamps below
// are expressed relative to it, in seconds, exactly as the real page reports
// them (`m.t` in seconds, `Date.now()` in milliseconds).
const pageNowMillis = 1787000000000

// row is a message row from the fake page, FRESH: the message's own timestamp
// is the same second the page announced it.
//
// IT CARRIES msgT BECAUSE THE REAL PAGE DOES. The first version of this helper
// omitted it, and the freshness classifier — which has no other evidence —
// correctly answered UNKNOWN for every row. Two tests then failed and accused
// the classifier of a defect it did not have: the double was less faithful than
// production, which is the mirror of the permissive-double trap this repository
// already catalogued.
func row(typ string, seq int64, msg string) string {
	return rowAged(typ, seq, msg, 0)
}

// rowAged is the same row for a message created ageSeconds BEFORE the page
// announced it, which is what hydration looks like.
func rowAged(typ string, seq int64, msg string, ageSeconds int64) string {
	return fmt.Sprintf(`{"type":%q,"seq":%d,"at":%d,"chat":"1@c.us","msg":%q,"fromMe":true,`+
		`"kind":"chat","ack":1,"bodyLen":7,"msgT":%d}`,
		typ, seq, pageNowMillis, msg, pageNowMillis/1000-ageSeconds)
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

// TestAQuietStartDoesNotMarkTheFirstREALEventAsReplay. The replay burst, if any,
// is already buffered when the first drain runs; if that drain is empty there
// was no replay, and a later event is not history.
//
// This was a live flake before it was a test: the same send produced
// message.added on one run and nothing on the next, because the subscriber
// skipped what the pump had mislabelled.
func TestAQuietStartDoesNotMarkTheFirstRealEventAsReplay(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}, rows: [][]string{
		{},                                // a quiet page: nothing buffered
		{row("message.added", 1, "REAL")}, // and then something happens
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
	if len(replay) != 0 {
		t.Fatalf("a quiet start marked real events as replay: %v", replay)
	}
	if len(live) != 1 || live[0] != "REAL" {
		t.Fatalf("the real event was not delivered live: %v", live)
	}
}

// TestTheHydrationBurstIsNotDeliveredAsLive is the defect this whole mechanism
// exists for, reproduced at the size it really has.
//
// Measured on a real boot (H93): ~1170 messages and ~2000 chat changes pour
// through in the first eight seconds, all of them hydration, and the old replay
// window closed after the FIRST drain. Everything after it was announced as
// news, so a consumer counting arrivals would have counted eleven hundred
// historical messages on every boot.
//
// The burst here is deliberately spread across MANY drains, because one drain is
// the only case the old window ever handled.
func TestTheHydrationBurstIsNotDeliveredAsLive(t *testing.T) {
	const drains, perDrain = 12, 40
	batches := make([][]string, 0, drains+1)
	seq := int64(0)
	for d := 0; d < drains; d++ {
		batch := make([]string, 0, perDrain)
		for i := 0; i < perDrain; i++ {
			seq++
			// Hydration: each message is DAYS old, whatever the clock says about
			// when the page got round to announcing it.
			batch = append(batch, rowAged("message.added", seq, "OLD", 86400*2))
		}
		batches = append(batches, batch)
	}
	// And then something genuinely happens.
	seq++
	batches = append(batches, []string{row("message.added", seq, "REAL")})

	f := &fakePage{freshAt: map[int]bool{1: true}, rows: batches}
	h := NewHub()
	var live, replay, unknown []string
	h.Subscribe(func(e Event) {
		switch e.Fresh {
		case FreshnessLive:
			live = append(live, e.MessageID)
		case FreshnessReplay:
			replay = append(replay, e.MessageID)
		default:
			unknown = append(unknown, e.MessageID)
		}
	})
	runPump(t, f, h, len(batches))

	if len(live) != 1 || live[0] != "REAL" {
		t.Fatalf("live = %v; the hydration burst must not contain a single live event, "+
			"and the one real message must be in there", live)
	}
	if len(replay) != drains*perDrain {
		t.Fatalf("%d of %d hydration events were classified as replay", len(replay), drains*perDrain)
	}
	if len(unknown) != 0 {
		t.Errorf("%d message.added events came back UNKNOWN; they all carry a timestamp "+
			"and are therefore classifiable", len(unknown))
	}
}

// A TYPE WITH NO CAUSAL DISCRIMINATOR SAYS SO. chat.changed carries nothing
// separating hydration from news, so it is UNKNOWN — never live, and never
// promoted to live because time passed or the burst subsided.
func TestATypeWithNoDiscriminatorStaysUnknown(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}, rows: [][]string{
		{row("chat.changed", 1, "")},
		{row("chat.changed", 2, ""), row("chat.changed", 3, "")},
	}}
	h := NewHub()
	var got []Freshness
	h.Subscribe(func(e Event) { got = append(got, e.Fresh) })
	runPump(t, f, h, 2)

	if len(got) != 3 {
		t.Fatalf("delivered %d events, want 3", len(got))
	}
	if got[0] != FreshnessReplay {
		t.Errorf("the first drain is %s, want %s", got[0], FreshnessReplay)
	}
	for _, f := range got[1:] {
		if f != FreshnessUnknown {
			t.Errorf("a chat.changed after the first drain is %s; there is nothing about it "+
				"that proves it is news", f)
		}
	}
}

// UNKNOWN READS AS REPLAY FOR EVERY OLD CONSUMER, and that direction is the one
// that cannot hurt: skipping something that was new costs a missed event, while
// counting eleven hundred historical messages as arrivals corrupts a total.
func TestUnknownIsConservativeForTheOldBoolean(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}, rows: [][]string{
		{},
		{row("chat.changed", 1, ""), row("message.added", 2, "REAL"),
			rowAged("message.added", 3, "OLD", 86400)},
	}}
	h := NewHub()
	seen := map[string]Event{}
	h.Subscribe(func(e Event) { seen[string(e.Fresh)] = e })
	runPump(t, f, h, 2)

	for _, want := range []Freshness{FreshnessUnknown, FreshnessLive, FreshnessReplay} {
		e, ok := seen[string(want)]
		if !ok {
			t.Fatalf("no %s event was delivered; the three states are not all reachable", want)
		}
		if wantReplay := want != FreshnessLive; e.Replay != wantReplay {
			t.Errorf("%s carries Replay=%t, want %t", want, e.Replay, wantReplay)
		}
	}
}

// A MESSAGE ANNOUNCED IN THE SAME SECOND IT WAS CREATED IS THE FRESHEST CASE
// THERE IS, and the first classifier called it UNKNOWN.
//
// It guarded on AgeSeconds > 0, which conflates "no timestamp" with "zero
// seconds old". A send and its own echo land in the same second routinely.
func TestAZeroSecondOldMessageIsLive(t *testing.T) {
	f := &fakePage{freshAt: map[int]bool{1: true}, rows: [][]string{
		{},
		{rowAged("message.added", 1, "INSTANT", 0)},
	}}
	h := NewHub()
	var got Freshness
	h.Subscribe(func(e Event) { got = e.Fresh })
	runPump(t, f, h, 2)
	if got != FreshnessLive {
		t.Fatalf("a message announced in the second it was created is %s, want %s", got, FreshnessLive)
	}
}

// THE WINDOW IS NOT A CLOCK. Nothing about the classifier's answer depends on
// when the pump ran, only on what the message says about itself — which is what
// separates this from the rate heuristic it replaces.
func TestFreshnessDoesNotDependOnWhenThePumpRan(t *testing.T) {
	if strings.Contains(classifierSource(t), "time.Now()") {
		t.Fatal("classify reads the clock; freshness would then depend on when the pump " +
			"happened to poll, which is the defect this replaces")
	}
}

// classifierSource returns the body of classify, so a test can assert about what
// it is allowed to consult.
func classifierSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("ingress.go")
	if err != nil {
		t.Fatalf("read ingress.go: %v", err)
	}
	src := string(body)
	i := strings.Index(src, "func classify(")
	if i < 0 {
		t.Fatal("classify is gone")
	}
	return src[i:]
}

// A RELOAD REPEATS THE CYCLE. The page's buffer and handlers are gone, the pump
// reinstalls, and the collections refill from history — so the first drain after
// a reinstall is replay again, exactly as it is after the first install.
//
// Without this, a session that reloads mid-life would announce its entire
// history a second time, and the freshness rule would have fixed the boot case
// only.
func TestAReloadRestartsTheReplayWindow(t *testing.T) {
	f := &fakePage{
		// Fresh installs at cycle 1 and again at cycle 3: a reload in between.
		freshAt: map[int]bool{1: true, 3: true},
		rows: [][]string{
			{rowAged("message.added", 1, "HIST1", 86400)},
			{row("message.added", 2, "REAL")},
			{rowAged("message.added", 3, "HIST2", 86400), row("message.added", 4, "REAL2")},
		},
	}
	h := NewHub()
	var got []Freshness
	var ids []string
	h.Subscribe(func(e Event) {
		got = append(got, e.Fresh)
		ids = append(ids, e.MessageID)
	})
	runPump(t, f, h, 3)

	if len(got) != 4 {
		t.Fatalf("delivered %v (%d events), want 4", ids, len(got))
	}
	// Drain 1 (first install), drain 3 (first after reinstall): replay.
	if got[0] != FreshnessReplay {
		t.Errorf("the first drain is %s, want %s", got[0], FreshnessReplay)
	}
	if got[1] != FreshnessLive {
		t.Errorf("the event between the installs is %s, want %s", got[1], FreshnessLive)
	}
	// THE WHOLE DRAIN AFTER A REINSTALL IS REPLAY, including the message that
	// would otherwise read as fresh — because after a reload the page cannot
	// distinguish what it is refilling from what just happened, and neither can
	// this bus.
	if got[2] != FreshnessReplay || got[3] != FreshnessReplay {
		t.Errorf("the drain after the reinstall is %s/%s, want both %s",
			got[2], got[3], FreshnessReplay)
	}
}
