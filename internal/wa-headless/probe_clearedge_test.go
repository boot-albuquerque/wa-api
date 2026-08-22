package waheadless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeClearEdge measures the case that decides the SHAPE of decision 67's
// postcondition, before any of it is written.
//
// The decision says Clear must "provar redução e falhar quando a pós-condição não
// ocorrer". The naive encoding — after < before — has an edge that would make it
// wrong: a conversation holding ONLY system notifications. H166 measured 4 -> 1
// with an e2e_notification surviving, so system messages are not cleared. If a
// chat has nothing BUT those, a correct no-op would read as a failure.
//
// A THROWAWAY GROUP IS EXACTLY THAT CHAT: freshly created, it holds only what the
// system put there. So this measures the edge directly instead of reasoning about
// it — and then measures the ordinary case in the same group, for contrast.
func TestProbeClearEdge(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CLEAREDGE") == "" {
		t.Skip("set WA_PROBE_CLEAREDGE=1 (creates a throwaway group and clears it twice)")
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
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate
	g, c := group.New(runner, eval), chats.New(runner, eval)

	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/clearedge/resolve")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}
	subject := "wa-headless clear-edge probe " + strconv.FormatInt(time.Now().Unix(), 10)
	created, err := g.Ensure(ctx, subject, []string{ident.JID}, "probe/clearedge/create")
	if err != nil {
		t.Fatalf("creating the throwaway group: %v", err)
	}
	gjid := created.JID
	defer func() {
		_ = g.Leave(context.Background(), gjid, "probe/clearedge/leave")
		_ = c.Delete(context.Background(), gjid, "probe/clearedge/delete")
	}()
	if !created.Created {
		t.Fatal("Ensure matched an existing group instead of creating one")
	}

	shapes := func(what string) string {
		var raw string
		script := `(() => {
			try {
				const out = {};
				let n = 0;
				for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
					const r = m.id && m.id.remote;
					const s = (r && r._serialized) ? r._serialized : r;
					if (s !== ` + strconv.Quote(gjid) + `) { continue; }
					n++;
					const k = String(m.type || "?");
					out[k] = (out[k] || 0) + 1;
				}
				return JSON.stringify({total: n, byType: out});
			} catch (e) { return JSON.stringify({err: String(e).slice(0,80)}); }
		})()`
		if err := eval(ctx, script, &raw); err != nil {
			t.Fatalf("counting (%s): %v", what, err)
		}
		return raw
	}

	time.Sleep(4 * time.Second)
	t.Logf("A) grupo recem-criado, ANTES de qualquer envio: %s", shapes("virgin"))

	// A BORDA: limpar um chat que so' tem sistema.
	e1, err := c.Clear(ctx, gjid, false, "probe/clearedge/clear-virgin")
	t.Logf("A) Clear: %v err=%v", e1, err)
	time.Sleep(5 * time.Second)
	t.Logf("A) DEPOIS: %s", shapes("virgin-after"))

	// O CASO ORDINARIO, no mesmo grupo, para contraste.
	for i := 0; i < 2; i++ {
		if _, err := send.Text(ctx, runner, eval, gjid, "wa-headless clear-edge "+strconv.Itoa(i), "probe/clearedge/send"); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	time.Sleep(4 * time.Second)
	t.Logf("B) com mensagens, ANTES: %s", shapes("loaded"))
	e2, err := c.Clear(ctx, gjid, false, "probe/clearedge/clear-loaded")
	t.Logf("B) Clear: %v err=%v", e2, err)
	time.Sleep(5 * time.Second)
	t.Logf("B) DEPOIS: %s", shapes("loaded-after"))

	// C) A BORDA DE VERDADE: limpar o que JA' esta limpo. Nos dois casos acima o
	// Clear reduziu, entao "after < before" funcionaria — mas so' porque sempre
	// havia algo a tirar. Se aqui nao reduzir, uma pos-condicao ingenua
	// reportaria FALHA num no-op correto, e a decisao 67 viraria um defeito.
	t.Logf("C) ja limpo, ANTES: %s", shapes("clean"))
	e3, err := c.Clear(ctx, gjid, false, "probe/clearedge/clear-again")
	t.Logf("C) Clear: %v err=%v", e3, err)
	time.Sleep(5 * time.Second)
	t.Logf("C) DEPOIS: %s", shapes("clean-after"))

	// A POS-CONDICAO TEM DE ACEITAR OS TRES CASOS MEDIDOS. Um erro em qualquer um
	// deles significa que a regra da decisao 67 ficou mais estreita que o mundo.
	if err != nil {
		t.Fatalf("C) clearing an already-clear chat FAILED (%v); the postcondition "+
			"is narrower than the world it measures", err)
	}
	if e1.MessagesAfter <= 0 || e2.MessagesAfter <= 0 || e3.MessagesAfter <= 0 {
		t.Fatalf("MessagesAfter came back as zero somewhere (%d/%d/%d); a clear "+
			"always leaves the system notification, and reporting zero would hide it",
			e1.MessagesAfter, e2.MessagesAfter, e3.MessagesAfter)
	}
	t.Logf("PROVEN (67): os tres casos passam — virgem %d->%d, com mensagens %d->%d, "+
		"ja limpo %d->%d", e1.MessagesBefore, e1.MessagesAfter,
		e2.MessagesBefore, e2.MessagesAfter, e3.MessagesBefore, e3.MessagesAfter)
}
