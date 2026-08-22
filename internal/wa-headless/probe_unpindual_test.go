package waheadless

import (
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/pin"
	"wa-api/internal/wa-headless/capabilities/send"
)

// TestProbeUnpinAcrossSessions re-measures a BLOCKED row whose justification was
// invalidated by today's own findings.
//
// `unpin` is BLOCKED on "idem pin — chamada aceita, nada desfixado (H81)". H162
// showed that H81's verdict about the PIN was measured from the only side that
// cannot see it: the acting session does not show its own pin, so "it worked and
// I cannot see it" and "it did nothing" read identically there. conta-B saw the
// pin. The inherited verdict for unpin was never re-checked.
//
// Closing Phase 1 with a justification that this week's work refuted would be
// exactly the "achado com diagnóstico errado" the project calls worse than none.
// TestProbeLabPinState reports how many messages are pinned in the lab group and
// attempts to clear them. It exists because TestProbeUnpinAcrossSessions LEFT ONE
// PINNED: the unpin does not take, which is the very thing it measured.
func TestProbeLabPinState(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PINSTATE") == "" {
		t.Skip("set WA_PROBE_PINSTATE=1")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	if pa == "" || pb == "" {
		t.Fatal("WA_PROFILE_A and WA_PROFILE_B are required")
	}
	d, ctx, done := openDual(t, pa, pb, 10*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate
	gjid := findLabGroupJID(ctx, t, d.RunnerA, evalA)
	if gjid == "" {
		t.Skip("lab group not found")
	}
	pA, pB := pin.New(d.RunnerA, evalA), pin.New(d.RunnerB, evalB)

	seen, err := pB.PinnedIn(ctx, gjid, "probe/pinstate/read")
	if err != nil {
		t.Fatalf("conta-B reading pins: %v", err)
	}
	t.Logf("lab group currently shows %d pinned message(s) to conta-B", len(seen))
	for _, id := range seen {
		res, err := pA.Unpin(ctx, id, "probe/pinstate/clear")
		t.Logf("  unpin attempt: %v err=%v", res, err)
	}
	if len(seen) == 0 {
		return
	}
	time.Sleep(10 * time.Second)
	after, err := pB.PinnedIn(ctx, gjid, "probe/pinstate/verify")
	t.Logf("after the attempts, conta-B shows %d pinned (err=%v)", len(after), err)
	if len(after) > 0 {
		t.Logf("NOT CLEARED: the unpin does not take on this build, which is the " +
			"measured fact behind the BLOCKED row. The pin expires on its own " +
			"(the pin call reported seconds=604800, i.e. 7 days).")
	}
}

func TestProbeUnpinAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_UNPINDUAL") == "" {
		t.Skip("set WA_PROBE_UNPINDUAL=1 (pins and unpins a message in the lab group)")
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
		t.Skip("lab group not found")
	}
	pA, pB := pin.New(d.RunnerA, evalA), pin.New(d.RunnerB, evalB)

	// A LINHA DE BASE VEM DO OBSERVADOR (H162).
	before, err := pB.PinnedIn(ctx, gjid, "probe/unpin/before")
	if err != nil {
		t.Fatalf("conta-B reading pins: %v", err)
	}
	t.Logf("BEFORE (conta-B): %d pinned", len(before))

	sent, err := send.Text(ctx, d.RunnerA, evalA, gjid, "wa-headless unpin probe", "probe/unpin/send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	time.Sleep(5 * time.Second)
	if _, err := pA.Message(ctx, sent.ID.ID, "probe/unpin/pin"); err != nil {
		t.Fatalf("pin: %v", err)
	}

	// O PIN TEM DE APARECER PARA CONTA-B ANTES DE DESFAZER. Desfazer o que nunca
	// apareceu nao mede o unpin — mede o pin de novo, e daria "sucesso" pelo
	// motivo errado.
	deadline := time.Now().Add(60 * time.Second)
	pinned := false
	for {
		mid, err := pB.PinnedIn(ctx, gjid, "probe/unpin/mid")
		if err == nil && len(mid) > len(before) {
			t.Logf("conta-B SEES the pin: %d -> %d", len(before), len(mid))
			pinned = true
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the pin never reached conta-B; the unpin cannot be measured from here")
		}
		time.Sleep(3 * time.Second)
	}
	_ = pinned

	unpinned, err := pA.Unpin(ctx, sent.ID.ID, "probe/unpin/unpin")
	if err != nil {
		t.Fatalf("Unpin: %v", err)
	}
	t.Logf("conta-A Unpin: %v", unpinned)

	// A JANELA E' LONGA PORQUE A CURTA MENTIU. A primeira versao deu 60s e
	// concluiu bloqueio; minutos depois, uma leitura independente mostrou 0
	// fixados — o unpin tinha pegado, so' mais devagar que o prazo. O prazo era o
	// instrumento, e um instrumento curto demais produz o mesmo texto que um
	// bloqueio de verdade.
	deadline = time.Now().Add(5 * time.Minute)
	start := time.Now()
	for {
		after, err := pB.PinnedIn(ctx, gjid, "probe/unpin/after")
		if err == nil && len(after) == len(before) {
			t.Logf("NEWS: conta-B sees the pin GONE (%d) after %s. unpin WORKS; its "+
				"BLOCKED verdict was inherited from H81, which H162 refuted.",
				len(after), time.Since(start).Round(time.Second))
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the unpin returned and conta-B still sees %d pinned (was %d "+
				"before the pin); the BLOCKED verdict stands, now from the side that "+
				"had never been asked", len(after), len(before))
		}
		time.Sleep(3 * time.Second)
	}
}
