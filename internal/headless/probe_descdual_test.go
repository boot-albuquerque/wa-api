package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
)

// TestProbeDescriptionAcrossSessions applies H162's lesson to the row the audit
// it produced points at first.
//
// setDescription is BLOCKED on "a página ACEITA e o servidor nunca armazena",
// measured in a channel (H113), independently in a group (H126), and reproduced
// by me in H145 — three times, and ALL THREE from the session that made the
// change. That is precisely the blind spot H162 just found in the pin: within the
// acting session, "it worked and I cannot see it" and "it did nothing" read the
// same, so repeating the measurement from that side cannot resolve it.
//
// conta-A writes the description; conta-B reads the group. The old subject and
// description are restored in a deferred step.
func TestProbeDescriptionAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_DESCDUAL") == "" {
		t.Skip("set WA_PROBE_DESCDUAL=1 (writes a description on the lab group)")
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
	gA, gB := group.New(d.RunnerA, evalA), group.New(d.RunnerB, evalB)

	// A LINHA DE BASE VEM DO OBSERVADOR, pela razão da H162: é a leitura dele
	// que decide, então é dela que o "antes" precisa vir.
	beforeB, err := gB.Metadata(ctx, gjid, "probe/descdual/before-B")
	if err != nil {
		t.Fatalf("conta-B reading the group: %v", err)
	}
	t.Logf("BEFORE (conta-B): descLen=%d source=%q", len(beforeB.Description), beforeB.DescriptionSource)

	const want = "wa-headless cross-session description probe"
	set, err := gA.SetDescription(ctx, gjid, want, "probe/descdual/set")
	// O ERRO NAO ENCERRA A MEDICAO. A pos-condicao do SetDescription le do lado
	// que a H162 mostrou ser cego; falhar ali e' compativel com "nao fez" E com
	// "fez e nao vejo".
	t.Logf("conta-A SetDescription: %v err=%v", set, err)

	defer func() {
		if beforeB.Description == "" {
			t.Log("RESTORE: the group had no description before; leaving the probe " +
				"text would be a change nobody asked for, but clearing needs the " +
				"same write that is under test — reported, not hidden")
		}
		if _, err := gA.SetDescription(context.Background(), gjid, beforeB.Description, "probe/descdual/restore"); err != nil {
			t.Logf("RESTORE returned %v (expected if the write reports failure; the "+
				"cross-session read below says what actually happened)", err)
		}
	}()

	deadline := time.Now().Add(60 * time.Second)
	for {
		afterB, err := gB.Metadata(ctx, gjid, "probe/descdual/after-B")
		if err == nil && afterB.Description == want {
			t.Logf("NEWS: conta-B SEES the description (source=%q). The server DOES "+
				"store it; H113/H126/H145 all measured from the blind side, and the "+
				"BLOCKED rows need re-measuring.", afterB.DescriptionSource)
			return
		}
		if time.Now().After(deadline) {
			t.Logf("CONFIRMED: conta-B does not see it either (descLen=%d source=%q "+
				"err=%v). setDescription stands BLOCKED, now from the side that had "+
				"never been asked.", len(afterB.Description), afterB.DescriptionSource, err)
			return
		}
		time.Sleep(3 * time.Second)
	}
}
