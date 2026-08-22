package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/block"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeBlocklistNamed proves block.List by producing the entry it needs and
// then TAKING IT BACK, which is the pattern the channel backup test established
// (H27): a proof that leaves the fixture changed has bought its evidence with
// somebody else's future test.
//
// The blocklist reads 0 on this account (H146), so an empty list proves nothing
// — it is what a broken reader returns too. The lab peer is blocked, the list is
// read, and the peer is unblocked in a deferred restore that runs even when the
// assertions fail.
func TestProbeBlocklistNamed(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_BLOCKLIST2") == "" {
		t.Skip("set WA_PROBE_BLOCKLIST2=1 (blocks the lab peer, then unblocks it)")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate
	b := block.New(runner, eval)

	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/blocklist2")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}

	before, err := b.List(ctx, "probe/blocklist2/before")
	if err != nil {
		t.Fatalf("List before: %v", err)
	}
	t.Logf("BEFORE: %d blocked", len(before))

	res, err := b.Block(ctx, peer, "probe/blocklist2/block")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	// O DESBLOQUEIO E' AGENDADO ANTES DE QUALQUER ASSERCAO. Registrar o restauro
	// depois das verificacoes deixaria o par bloqueado sempre que uma delas
	// falhasse — e a primeira falha e' justamente quando o fixture importa.
	defer func() {
		if _, err := b.Unblock(context.Background(), peer, "probe/blocklist2/restore"); err != nil {
			t.Errorf("RESTORE FAILED, the lab peer is still blocked: %v", err)
			return
		}
		back, err := b.List(context.Background(), "probe/blocklist2/verify-restore")
		if err != nil {
			t.Errorf("could not verify the restore: %v", err)
			return
		}
		if len(back) != len(before) {
			t.Errorf("the blocklist did not return to its original size: %d -> %d",
				len(before), len(back))
			return
		}
		t.Logf("RESTORED: back to %d blocked", len(back))
	}()
	t.Logf("Block: %s", res)

	during, err := b.List(ctx, "probe/blocklist2/during")
	if err != nil {
		t.Fatalf("List during: %v", err)
	}
	t.Logf("DURING: %d blocked", len(during))
	if len(during) != len(before)+1 {
		t.Fatalf("the list did not grow by exactly one: %d -> %d", len(before), len(during))
	}
	var named bool
	for _, j := range during {
		if j == ident.JID || j == peer {
			named = true
		}
	}
	if !named {
		t.Fatal("the blocklist grew and does not name the identity that was blocked, " +
			"so the entries are not the identities a caller would unblock")
	}
	t.Log("PROVEN: the list names who is blocked")
}
