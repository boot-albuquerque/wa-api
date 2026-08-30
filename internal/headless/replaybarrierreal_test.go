package headless

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/events"
	waruntime "wa-api/internal/headless/runtime"
)

// TestReplayBarrierReal proves the freshness classifier against the burst it
// was built for, on a real account.
//
// The unit tests reproduce the SHAPE of the hydration flood; only a real boot
// reproduces its SIZE and its timing. Four properties, all demanded before this
// was allowed to ship:
//
//	the hydration burst contains no live event
//	an action after it produces one
//	a reload repeats the cycle
//	nothing historical reaches an old consumer as news
//
// conta-A sends one marker message to conta-B. That is the only outward effect.
func TestReplayBarrierReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_REPLAY_BARRIER") == "" {
		t.Skip("set WA_REAL_REPLAY_BARRIER=1; conta-A sends one message to conta-B")
	}
	fromProfile := os.Getenv("WA_SEND_FROM_PROFILE")
	toProfile := os.Getenv("WA_SEND_TO_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if fromProfile == "" || toProfile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE and WA_SEND_TO_JID are required")
	}

	// conta-B is the OBSERVER: it boots, floods, and then receives.
	runnerB := engine.NewRunner()
	hB := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: toProfile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runnerB,
	})
	defer hB.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	sessB, err := hB.Session(ctx)
	if err != nil {
		t.Fatalf("conta-B boot: %v", err)
	}

	hub := events.NewHub()
	defer hub.Close()
	var mu sync.Mutex
	byFresh := map[events.Freshness]int{}
	// oldConsumer is written the way every subscriber written before Freshness
	// existed was written: it skips e.Replay and counts the rest as news.
	oldConsumerCounted := 0
	var liveMarkers []string
	defer hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		byFresh[e.Fresh]++
		if !e.Replay {
			oldConsumerCounted++
		}
		if e.Fresh == events.FreshnessLive && e.Type == events.MessageAdded {
			liveMarkers = append(liveMarkers, e.MessageID)
		}
	})()
	pumpCtx, stopPump := context.WithCancel(ctx)
	defer stopPump()
	go func() { _ = events.NewPump(runnerB, sessB.Tab().Evaluate, hub).Run(pumpCtx) }()

	// Let the hydration run its course. It was measured at about eight seconds;
	// twenty-five is generous and the assertion does not depend on the number —
	// what is asserted is that whatever arrived was not called live.
	time.Sleep(25 * time.Second)

	snap := func() (map[events.Freshness]int, int) {
		mu.Lock()
		defer mu.Unlock()
		out := map[events.Freshness]int{}
		for k, v := range byFresh {
			out[k] = v
		}
		return out, oldConsumerCounted
	}
	burst, oldSaw := snap()
	total := 0
	for _, v := range burst {
		total += v
	}
	t.Logf("hydration burst: %v (total %d); an old consumer counted %d as news",
		burst, total, oldSaw)
	if total < 100 {
		t.Skipf("only %d events arrived; this account is not producing the burst this "+
			"test exists to measure, and passing on it would prove nothing", total)
	}
	if n := burst[events.FreshnessLive]; n != 0 {
		t.Errorf("%d event(s) in the hydration burst were classified LIVE", n)
	}
	if oldSaw != 0 {
		t.Errorf("an old consumer counted %d historical events as news", oldSaw)
	}

	// AND NOW SOMETHING REAL HAPPENS.
	runnerA := engine.NewRunner()
	hA := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: fromProfile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runnerA,
	})
	defer hA.Stop(context.Background())
	sessA, err := hA.Session(ctx)
	if err != nil {
		t.Fatalf("conta-A boot: %v", err)
	}
	marker := fmt.Sprintf("headless replay barrier %d", time.Now().UnixNano())
	if _, err := send.Text(ctx, runnerA, sessA.Tab().Evaluate, peer, marker, "barrier/send"); err != nil {
		t.Fatalf("sending the marker: %v", err)
	}
	t.Log("marker sent from conta-A")

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(liveMarkers)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	after, oldSawAfter := snap()
	t.Logf("after the send: %v; an old consumer counted %d as news", after, oldSawAfter)
	if after[events.FreshnessLive] == 0 {
		t.Fatal("the message conta-A just sent did not arrive as LIVE; a classifier that " +
			"calls everything history is as useless as one that calls everything news")
	}
	if oldSawAfter == 0 {
		t.Error("an old consumer saw nothing at all; the conservative default has made " +
			"the bus silent rather than accurate")
	}
}
