package waheadless

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/capabilities/pin"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbePinnedReader settles whether getPinnedMessages is work left undone or
// the shadow of a BLOCKED writer — the distinction the Phase 1 criterion needs
// and that the row does not carry.
//
// The row says the reader works and the account has nothing pinned, and that
// proving non-empty needs pinning, "which is blocked (H81)". That reads as a
// deduction from a neighbouring row rather than a measurement, and H145 showed
// the same shape is worth making explicit: for `description` the writer was
// re-measured, still refused, and the reader's open half was then recorded as
// provably NOT ACTIONABLE by this module.
//
// This does the same for pins: send a message, try to pin it, and let the
// outcome decide. If the write still fails, the reader has no producer here.
func TestProbePinnedReader(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PINNED") == "" {
		t.Skip("set WA_PROBE_PINNED=1 (sends a message and attempts to pin it)")
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
	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/pinned/resolve")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}

	p := pin.New(runner, eval)
	// A LINHA DE BASE ANTES DE MEXER, e ela e' o proprio leitor: se vier algo
	// aqui, a premissa da linha ("a conta nao tem NADA fixado") ja caiu.
	before, err := p.PinnedIn(ctx, ident.JID, "probe/pinned/before")
	t.Logf("BEFORE: %d pinned, err=%v", len(before), err)
	if err == nil && len(before) > 0 {
		t.Fatalf("the account DOES have %d pinned message(s); the row's premise is "+
			"stale and the reader can be proven non-empty without pinning anything", len(before))
	}

	sent, err := send.Text(ctx, runner, eval, peer, "wa-headless pin probe", "probe/pinned/send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := p.Message(ctx, sent.ID.ID, "probe/pinned/pin")
	t.Logf("pin.Message: %v err=%v", got, err)
	// O SINAL DO BLOQUEIO NAO E' UM ERRO, e supor que fosse foi o defeito da
	// primeira versao desta sonda. A capacidade devolve nil e `verified=false`,
	// que e' ela dizendo com todas as letras "a pagina aceitou e eu nao consigo
	// confirmar" — a pos-condicao da H81. Ler so' o erro conta um aceite como
	// sucesso, que e' precisamente o que a invariante 14 existe para impedir.
	if err != nil || !got.Verified {
		t.Logf("the write still refuses, as H81 measured; the reader's open half " +
			"therefore has NO PRODUCER inside this module and the row is not " +
			"actionable here")
		t.Skip("pin is BLOCKED; nothing can pin, so nothing can be read back")
	}

	// SE CHEGOU AQUI, a H81 mudou de estado e isso e' noticia maior que a linha.
	defer func() {
		if _, err := p.Unpin(context.Background(), sent.ID.ID, "probe/pinned/restore"); err != nil {
			t.Errorf("RESTORE FAILED, the probe message is still pinned: %v", err)
		}
	}()
	after, err := p.PinnedIn(ctx, ident.JID, "probe/pinned/after")
	if err != nil {
		t.Fatalf("PinnedIn after pinning: %v", err)
	}
	if len(after) <= len(before) {
		t.Fatalf("the pin was accepted and the reader did not move: %d -> %d",
			len(before), len(after))
	}
	t.Logf("NEWS: pinning WORKS now (%d -> %d). H81 and the BLOCKED row need "+
		"re-measuring, and this row can be closed.", len(before), len(after))
	_ = errors.Is
}
