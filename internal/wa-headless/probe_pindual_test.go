package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/pin"
	"wa-api/internal/wa-headless/capabilities/send"
)

// TestProbePinAcrossSessions asks the one question about pinning that no single
// session can answer.
//
// pin.Message is BLOCKED on H81: "chamada aceita e nada fixado". But the type's
// own doc says Verified is false FOR A REAL CHANGE, because "this build does not
// show the session its own pin" — so within the acting session, "it worked and I
// cannot see it" and "it did nothing" produce the identical reading. Measuring
// again from the same side would reproduce the ambiguity, not resolve it.
//
// THIS IS THE SAME SHAPE H135 RESOLVED for participant events: a zero that was
// about the ACTING session, not about the world. conta-A pins a message in the
// shared lab group; conta-B reads the group's pinned list. conta-B has no stake
// in conta-A's local state, so what it sees is the server's answer.
//
// The message is pinned and then UNPINNED in a deferred restore, whatever the
// assertions do.
func TestProbePinAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PINDUAL") == "" {
		t.Skip("set WA_PROBE_PINDUAL=1 (pins a message in the lab group, then unpins it)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	if pa == "" || pb == "" {
		t.Fatal("WA_PROFILE_A and WA_PROFILE_B are required")
	}
	d, ctx, done := openDual(t, pa, pb, 12*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate

	gjid := findLabGroupJID(ctx, t, d.RunnerA, evalA)
	if gjid == "" {
		t.Skip("lab group not found; a shared conversation is what makes this work")
	}

	pA, pB := pin.New(d.RunnerA, evalA), pin.New(d.RunnerB, evalB)

	// A LINHA DE BASE E' LIDA DO OBSERVADOR, nao do ator. E' a leitura do
	// observador que vai decidir, entao e' dela que o "antes" precisa vir.
	before, err := pB.PinnedIn(ctx, gjid, "probe/pindual/before")
	if err != nil {
		t.Fatalf("conta-B reading the group's pins: %v", err)
	}
	t.Logf("BEFORE (seen by conta-B): %d pinned", len(before))

	sent, err := send.Text(ctx, d.RunnerA, evalA, gjid, "wa-headless pin-dual probe", "probe/pindual/send")
	if err != nil {
		t.Fatalf("conta-A send: %v", err)
	}
	time.Sleep(5 * time.Second)

	got, err := pA.Message(ctx, sent.ID.ID, "probe/pindual/pin")
	if err != nil {
		t.Fatalf("conta-A pin: %v", err)
	}
	t.Logf("conta-A pin.Message: %v", got)
	defer func() {
		if _, err := pA.Unpin(context.Background(), sent.ID.ID, "probe/pindual/restore"); err != nil {
			t.Errorf("RESTORE FAILED, the probe message may still be pinned: %v", err)
			return
		}
		t.Log("RESTORED: unpin issued")
	}()

	// O OBSERVADOR DECIDE.
	deadline := time.Now().Add(60 * time.Second)
	for {
		after, err := pB.PinnedIn(ctx, gjid, "probe/pindual/after")
		if err == nil && len(after) > len(before) {
			t.Logf("NEWS: conta-B SEES the pin (%d -> %d). H81's 'nada fixado' was "+
				"about the acting session, not about the server — pin WORKS and the "+
				"BLOCKED row needs re-measuring.", len(before), len(after))
			return
		}
		if time.Now().After(deadline) {
			t.Logf("CONFIRMED: conta-B does not see the pin either (%d, was %d, err=%v). "+
				"H81 stands, from the one side that had never been asked, and the "+
				"reader's open half has no producer here.", len(after), len(before), err)
			return
		}
		time.Sleep(3 * time.Second)
	}
}
