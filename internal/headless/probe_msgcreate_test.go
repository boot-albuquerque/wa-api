package headless

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/events"
)

// TestProbeMessageCreateBothDirections settles whether MESSAGE_CREATE is a
// missing event or an already-delivered one described wrongly.
//
// The row says "o mesmo evento cobre os dois; o upstream distingue criada de
// recebida e nós não". The upstream code says otherwise about HOW it
// distinguishes them (client.js:648-664): it emits MESSAGE_CREATE for every
// message, then `if (msg.id.fromMe) return;` and emits MESSAGE_RECEIVED for the
// rest. The only discriminator is fromMe — the exact field this module's
// message.added already carries.
//
// So message.added IS MESSAGE_CREATE, one to one, and MESSAGE_RECEIVED is that
// same event filtered. What was never PROVEN is the part that makes the claim
// real: that both values of FromMe actually arrive on this bus. One direction
// proves nothing, because an event that hardcoded FromMe would pass it.
func TestProbeMessageCreateBothDirections(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MSGCREATE") == "" {
		t.Skip("set WA_PROBE_MSGCREATE=1 (one message in each direction between the lab accounts)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	peerB, selfA := os.Getenv("WA_PEER_B_JID"), os.Getenv("WA_SELF_A_JID")
	if pa == "" || pb == "" || peerB == "" || selfA == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B, WA_PEER_B_JID and WA_SELF_A_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 10*time.Minute)
	defer done()

	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate

	// O BARRAMENTO FICA EM CONTA-A, e as duas mensagens sao vistas por ele: a
	// que A manda (FromMe verdadeiro) e a que B manda (FromMe falso).
	hub := events.NewHub()
	var mu sync.Mutex
	mine, theirs := 0, 0
	unsub := hub.Subscribe(func(e events.Event) {
		if e.Type != events.MessageAdded {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if e.FromMe {
			mine++
		} else {
			theirs++
		}
	})
	defer unsub()
	pump := events.NewPump(d.RunnerA, evalA, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()
	time.Sleep(6 * time.Second)

	mu.Lock()
	baseMine, baseTheirs := mine, theirs
	mu.Unlock()
	t.Logf("baseline: fromMe=%d notFromMe=%d", baseMine, baseTheirs)

	if _, err := send.Text(ctx, d.RunnerA, evalA, peerB, "headless create probe A->B", "probe/msgcreate/a"); err != nil {
		t.Fatalf("A->B: %v", err)
	}
	if _, err := send.Text(ctx, d.RunnerB, evalB, selfA, "headless create probe B->A", "probe/msgcreate/b"); err != nil {
		t.Fatalf("B->A: %v", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for {
		mu.Lock()
		gm, gt := mine-baseMine, theirs-baseTheirs
		mu.Unlock()
		if gm > 0 && gt > 0 {
			t.Logf("PROVEN: message.added carried BOTH values of FromMe (own=%d, incoming=%d)", gm, gt)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only one direction arrived (own=%d, incoming=%d); with a single "+
				"value of FromMe the event cannot be claimed to distinguish them", gm, gt)
		}
		time.Sleep(1 * time.Second)
	}
}
