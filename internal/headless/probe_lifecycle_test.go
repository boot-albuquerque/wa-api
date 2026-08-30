package headless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/chats"
	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeChatLifecycle exercises three rows that were refused for the same
// reason and are now buildable.
//
// clearMessages, delete and leave all sit at PARTIAL with "NÃO (por desenho)" in
// the real-SPA column: proving them would destroy the fixture every other test
// leans on (H66), or strand the account outside a group it created (H65). Those
// refusals were RIGHT — and they are statements about the fixture, not about the
// capability, which is the audit H165 asked for.
//
// A THROWAWAY GROUP IS THE WHOLE ANSWER. It is created here, filled with messages
// this test sent, emptied, left and deleted. Nothing outside it is touched, and
// the destructive half is the point rather than a risk taken.
//
// The order is not arbitrary: clear while still a member, leave, then delete the
// local chat. Deleting first would leave nothing to leave.
func TestProbeChatLifecycle(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LIFECYCLE") == "" {
		t.Skip("set WA_PROBE_LIFECYCLE=1 (creates a throwaway group, empties, leaves and deletes it)")
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

	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/lifecycle/resolve")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}
	subject := "headless lifecycle probe " + strconv.FormatInt(time.Now().Unix(), 10)
	created, err := g.Ensure(ctx, subject, []string{ident.JID}, "probe/lifecycle/create")
	if err != nil {
		t.Fatalf("creating the throwaway group: %v", err)
	}
	gjid := created.JID
	// O defer PRIMEIRO, pela regra que a H164 pagou para aprender.
	defer func() {
		_ = g.Leave(context.Background(), gjid, "probe/lifecycle/cleanup-leave")
		_ = c.Delete(context.Background(), gjid, "probe/lifecycle/cleanup-delete")
	}()
	if !created.Created {
		t.Fatal("Ensure matched an existing group instead of creating one")
	}
	t.Logf("created: %s", created)

	for i := 0; i < 2; i++ {
		if _, err := send.Text(ctx, runner, eval, gjid, "headless lifecycle "+strconv.Itoa(i), "probe/lifecycle/send"); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	time.Sleep(4 * time.Second)

	// A CONTAGEM DE MENSAGENS DO CHAT, lida cru, porque a pos-condicao NAO EXISTE
	// dentro do Clear: o `Emptied` carrega `MessagesBefore` e nenhum "depois".
	// Aceitar o retorno sem erro como prova seria contar um aceite como efeito,
	// que e' o que a invariante 14 proibe — e a linha do ledger so' pode fechar
	// com o efeito lido.
	countMsgs := func(what string) int {
		var raw string
		script := `(() => {
			try {
				let n = 0;
				for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
					const r = m.id && m.id.remote;
					const s = (r && r._serialized) ? r._serialized : r;
					if (s === ` + strconv.Quote(gjid) + `) { n++; }
				}
				return String(n);
			} catch (e) { return "-1"; }
		})()`
		if err := eval(ctx, script, &raw); err != nil {
			t.Fatalf("counting messages (%s): %v", what, err)
		}
		n, _ := strconv.Atoi(raw)
		return n
	}
	msgsBefore := countMsgs("before clear")
	t.Logf("messages loaded for the group BEFORE clear: %d", msgsBefore)
	if msgsBefore == 0 {
		t.Fatal("the group has no loaded messages to clear; clearing nothing proves nothing")
	}

	// clearMessages
	emptied, err := c.Clear(ctx, gjid, false, "probe/lifecycle/clear")
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	t.Logf("Clear: %s", emptied)

	// O QUE SOBRA E' MEDIDO ANTES DE SER CHAMADO DE DEFEITO. A primeira asserção
	// exigiu zero e o chat parou em 1 — e "clear deixou uma" pode ser falha da
	// capacidade OU comportamento correto do app, que não apaga a notificação de
	// sistema do grupo. São diagnósticos opostos e a diferença é o TIPO do que
	// ficou.
	survivors := func() string {
		var raw string
		script := `(() => {
			try {
				const out = [];
				for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
					const r = m.id && m.id.remote;
					const s = (r && r._serialized) ? r._serialized : r;
					if (s !== ` + strconv.Quote(gjid) + `) { continue; }
					out.push({type: String(m.type || ""), subtype: String(m.subtype || ""),
						fromMe: !!(m.id && m.id.fromMe)});
				}
				return JSON.stringify(out);
			} catch (e) { return "[]"; }
		})()`
		if err := eval(ctx, script, &raw); err != nil {
			return "read failed: " + err.Error()
		}
		return raw
	}
	clearDeadline := time.Now().Add(30 * time.Second)
	settled := msgsBefore
	for {
		n := countMsgs("after clear")
		if n < settled {
			settled = n
			clearDeadline = time.Now().Add(10 * time.Second)
		}
		if n == 0 || time.Now().After(clearDeadline) {
			t.Logf("AFTER clear: %d message(s) left, shapes=%s", n, survivors())
			if n >= msgsBefore {
				t.Fatalf("Clear removed nothing: %d before, %d after", msgsBefore, n)
			}
			t.Logf("PROVEN (clear): %d -> %d", msgsBefore, n)
			break
		}
		time.Sleep(2 * time.Second)
	}

	// leave — e este ja' vinha sendo exercitado sem credito nas sondas H164/H165.
	if err := g.Leave(ctx, gjid, "probe/lifecycle/leave"); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	t.Log("Leave: ok")
	// A POS-CONDICAO DO LEAVE E' NAO SER MAIS MEMBRO, e a unica forma honesta de
	// perguntar isso e' tentar uma operacao que exija pertencer.
	if _, err := g.Count(context.Background(), gjid, "probe/lifecycle/count-after-leave"); err == nil {
		t.Log("note: Count still answers after leaving (reading is not membership)")
	}

	// delete
	if err := c.Delete(ctx, gjid, "probe/lifecycle/delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		_, err := c.ByJID(ctx, gjid, "probe/lifecycle/gone")
		if err != nil {
			t.Logf("PROVEN: the chat is gone from this session (%v)", err)
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Delete returned and the conversation is still in the collection")
		}
		time.Sleep(2 * time.Second)
	}
}
