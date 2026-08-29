package headless

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/revoke"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/events"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeDeleteForMe proves ForMe and MessageRemoved together, because neither
// can be proven alone: the event has no source until the method exists, and the
// method has no observable consequence until something listens.
//
// IT DELETES A MESSAGE IT JUST SENT, and nothing else. The message is created by
// this test seconds earlier and goes to the lab peer, so the destructive act
// lands on something this test owns — and it is local-only anyway, which is the
// whole point of the method under test.
func TestProbeDeleteForMe(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_DELME") == "" {
		t.Skip("set WA_PROBE_DELME=1 (sends a message to the lab peer, then deletes it locally)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	hub := events.NewHub()
	var mu sync.Mutex
	counts := map[events.Type]int{}
	removedIDs := map[string]bool{}
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		counts[e.Type]++
		if e.Type == events.MessageRemoved {
			removedIDs[e.MessageID] = true
		}
	})
	defer unsub()
	pump := events.NewPump(runner, eval, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()
	time.Sleep(6 * time.Second)

	sent, err := send.Text(ctx, runner, eval, peer, "wa-headless local delete probe", "probe/delme")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent: %s", sent)
	time.Sleep(3 * time.Second)

	// A LINHA DE BASE ANTES DE MEXER. Sem ela, um message.removed que ja' estava
	// chegando por despejo da colecao seria lido como consequencia do apagamento.
	mu.Lock()
	before := counts[events.MessageRemoved]
	mu.Unlock()
	t.Logf("message.removed before the delete: %d", before)

	res, err := revoke.New(runner, eval).ForMe(ctx, sent.ID.ID, true, "probe/delme")
	if err != nil {
		t.Fatalf("ForMe: %v", err)
	}
	t.Logf("ForMe: as=%s waited=%s", res.As, res.Waited.Round(time.Millisecond))

	deadline := time.Now().Add(30 * time.Second)
	for {
		mu.Lock()
		got, mine := counts[events.MessageRemoved], removedIDs[sent.ID.ID]
		mu.Unlock()
		if mine {
			t.Logf("message.removed fired for the deleted message (total %d, was %d)", got, before)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ForMe removed the message and no message.removed named it "+
				"(total %d, baseline %d)", got, before)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
