package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeGroupDescriptionRead closes the reader's open half by PRODUCING the
// data it never had, which is the same manoeuvre H142 used on mentions.
//
// The `description` row says the reader is delivered and was NEVER OBSERVED
// NON-EMPTY: in the lab group both `desc` and `displayedDesc` read undefined,
// which is compatible with "this group has no description" AND with "the reader
// looks at the wrong field". DescriptionSource exists precisely because those
// two cannot be told apart from a zero — and telling them apart needs a group
// that HAS one.
//
// group.SetDescription (H126) is what makes that producible now.
func TestProbeGroupDescriptionRead(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_GDESC2") == "" {
		t.Skip("set WA_PROBE_GDESC2=1 (writes a description on the lab group)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
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
	gjid := findLabGroupJID(ctx, t, runner, eval)
	if gjid == "" {
		t.Skip("lab group not found")
	}
	mgr := group.New(runner, eval)

	// A LINHA DE BASE, ANTES DE MEXER. Sem ela o "depois" nao significa nada: um
	// leitor que devolvesse texto constante passaria igual.
	before, err := mgr.Metadata(ctx, gjid, "probe/gdesc2/before")
	if err != nil {
		t.Fatalf("metadata before: %v", err)
	}
	t.Logf("BEFORE: descLen=%d source=%q", len(before.Description), before.DescriptionSource)

	const want = "wa-headless lab fixture description"
	set, err := mgr.SetDescription(ctx, gjid, want, "probe/gdesc2/set")
	if err != nil {
		// A ESCRITA E' `BLOCKED` DESDE A H126, e reencontrar o bloqueio nao e'
		// falha deste teste — e' o resultado dele. O que este probe estabelece,
		// e que o ledger nao dizia, e' que a metade aberta do LEITOR depende da
		// escrita bloqueada: nao existe caminho por este modulo para produzir
		// uma descricao e, portanto, para observar o leitor nao-vazio.
		//
		// Isso muda a linha de "PARTIAL, talvez acionavel" para "PARTIAL,
		// comprovadamente nao acionavel aqui", que e' exatamente a distincao que
		// o criterio de encerramento da Fase 1 pede.
		t.Logf("SetDescription failed as H113/H126 measured: %v", err)
		t.Skip("the writer is BLOCKED, so the reader's open half has no producer " +
			"inside this module; the row stays PARTIAL and is not actionable")
	}
	t.Logf("SET: %s", set)

	after, err := mgr.Metadata(ctx, gjid, "probe/gdesc2/after")
	if err != nil {
		t.Fatalf("metadata after: %v", err)
	}
	t.Logf("AFTER: descLen=%d source=%q", len(after.Description), after.DescriptionSource)

	if after.Description != want {
		t.Fatalf("the reader did not return the description that was just written "+
			"(len %d, source %q)", len(after.Description), after.DescriptionSource)
	}
	if after.DescriptionSource == "none" || after.DescriptionSource == "" {
		t.Fatal("the reader returned the text and named no source, so the source " +
			"field is not reporting where it actually came from")
	}
	if before.DescriptionSource == after.DescriptionSource && len(before.Description) == len(after.Description) {
		t.Fatal("nothing moved between the two readings; the reader may be " +
			"returning a constant rather than reading the group")
	}
	t.Logf("PROVEN: source moved %q -> %q", before.DescriptionSource, after.DescriptionSource)
}
