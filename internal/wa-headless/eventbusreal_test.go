package waheadless

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/events"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAEventBusFoundation proves the seven properties the bus has to have
// before any of the upstream's 31 events are mapped onto it.
//
// They are proven in ONE run against ONE session on purpose: several of them are
// about how the properties interact — a reinstall that does not duplicate is
// only meaningful if delivery worked before AND after it — and separate tests
// would each start from a clean page and prove the easy half.
func TestRealSPAEventBusFoundation(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_EVENTS_TEST") == "" {
		t.Skip("set WA_HEADLESS_EVENTS_TEST=1; this sends messages to the peer lab account")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// A short poll so the test does not spend its life waiting; the production
	// default is measured for round-trip cost, not for test patience.
	oldPoll := events.PollInterval
	events.PollInterval = 200 * time.Millisecond
	defer func() { events.PollInterval = oldPoll }()

	hub := events.NewHub()
	pump := events.NewPump(runner, sess.Tab().Evaluate, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	var pumpDone sync.WaitGroup
	pumpDone.Add(1)
	go func() { defer pumpDone.Done(); _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		pumpDone.Wait()
		if err := pump.Uninstall(context.Background()); err != nil {
			t.Errorf("uninstall: %v", err)
		}
	}()

	// A recorder that keeps only what an event carries — never content.
	type seenEvent struct {
		typ    events.Type
		msgID  string
		replay bool
	}
	var mu sync.Mutex
	var seen []seenEvent
	off := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, seenEvent{e.Type, e.MessageID, e.Replay})
	})

	waitFor := func(what string, pred func() bool, budget time.Duration) bool {
		t.Helper()
		deadline := time.Now().Add(budget)
		for time.Now().Before(deadline) {
			mu.Lock()
			ok := pred()
			mu.Unlock()
			if ok {
				return true
			}
			time.Sleep(150 * time.Millisecond)
		}
		t.Errorf("timed out waiting for %s", what)
		return false
	}
	countOf := func(id string) int {
		n := 0
		for _, e := range seen {
			if e.msgID == id && e.typ == events.MessageAdded {
				n++
			}
		}
		return n
	}

	// --- 1 and 2: single install, and LIVE delivery -----------------------
	sent, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer,
		fmt.Sprintf("wa-headless event probe %d", time.Now().UnixNano()), "events/send-1")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitFor("the sent message to arrive as an event", func() bool {
		return countOf(sent.ID.ID) > 0
	}, 20*time.Second)

	mu.Lock()
	live := 0
	for _, e := range seen {
		if e.msgID == sent.ID.ID && !e.replay {
			live++
		}
	}
	mu.Unlock()
	if live == 0 {
		t.Error("the message arrived only as REPLAY; a message sent after the pump started is live")
	}
	if s := hub.Stats(); s.Reinstalls != 1 {
		t.Errorf("the ingress was installed %d time(s) before any reload, want 1: %s", s.Reinstalls, s)
	}

	// --- 3: unsubscribe stops delivery ------------------------------------
	off()
	mu.Lock()
	countAtUnsubscribe := len(seen)
	mu.Unlock()

	second, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer,
		fmt.Sprintf("wa-headless event probe %d", time.Now().UnixNano()), "events/send-2")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	time.Sleep(4 * time.Second)
	mu.Lock()
	grew := len(seen) - countAtUnsubscribe
	mu.Unlock()
	if grew != 0 {
		t.Errorf("%d event(s) arrived after unsubscribe", grew)
	}
	_ = second

	// --- 4, 5: reload reinstalls, and does NOT duplicate ------------------
	before := hub.Stats().Reinstalls
	offAgain := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, seenEvent{e.Type, e.MessageID, e.Replay})
	})
	defer offAgain()

	if err := sess.Tab().Navigate(runner, realSPAURL, "events/reload"); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !waitFor("the ingress to be reinstalled after the reload", func() bool {
		return hub.Stats().Reinstalls > before
	}, 90*time.Second) {
		return
	}
	t.Logf("MEASURED: %s", hub.Stats())

	// THE INGRESS INSTALLS BEFORE THE PAGE CAN SEND, and that is a measured
	// ordering fact rather than a flake: the collections exist — which is all
	// the ingress needs — while comms is still starting, and a send in that
	// window fails with "[comms] sendIq called before startComms".
	//
	// So readiness to SEND is waited for separately from readiness to OBSERVE.
	// Conflating them would make the bus look broken because something else was
	// not ready yet.
	var third send.Result
	sendDeadline := time.Now().Add(90 * time.Second)
	for {
		third, err = send.Text(ctx, runner, sess.Tab().Evaluate, peer,
			fmt.Sprintf("wa-headless event probe %d", time.Now().UnixNano()), "events/send-3")
		if err == nil {
			break
		}
		if time.Now().After(sendDeadline) {
			t.Fatalf("send after reload never became possible: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
	waitFor("the post-reload message to arrive", func() bool {
		return countOf(third.ID.ID) > 0
	}, 30*time.Second)

	mu.Lock()
	dupes := countOf(third.ID.ID)
	mu.Unlock()
	// ONE SUBSCRIBER, ONE COPY. Two would mean the reload left the old handlers
	// attached and the reinstall added a second set — the exact failure a
	// self-healing install could cause and the reason it is idempotent.
	if dupes > 1 {
		t.Errorf("the post-reload message arrived %d times; the reload duplicated the handlers", dupes)
	}

	// --- 6: the page counts what it drops ---------------------------------
	s := hub.Stats()
	t.Logf("MEASURED: %s", s)
	if s.SeenInPage <= 0 {
		t.Error("the page reports having seen no events at all")
	}
	// The two drop counters are independent measurements of the same fact, and
	// a disagreement means one of them is lying.
	if s.DroppedInPage == 0 && s.Gaps != 0 {
		t.Errorf("Go counted %d gap(s) and the page reports no drops: %s", s.Gaps, s)
	}

	// --- 7: teardown, checked by the deferred Uninstall and this ----------
	hub.Close()
	if !hub.Closed() || hub.Stats().Subscribers != 0 {
		t.Errorf("Close left the hub alive: %s", hub.Stats())
	}
}
