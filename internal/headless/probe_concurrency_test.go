package headless

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/message"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeParkedStateUnderConcurrency measures a hazard that is visible in the
// code and was never measured: EVERY reader in a capability parks its answer on
// the SAME page global.
//
// capabilities/message has one stateKey — "__headlessMessage" — shared by
// OriginOf, ShapeOf, CurrentOf, QuotedOf, MentionsOf, InfoOf and ReactionsOf.
// Two concurrent calls on one session write the same variable, and each polls it
// until non-empty. Whoever reads first can read the OTHER call's answer.
//
// WHY THIS IS A FASE 2 ITEM AND NOT A FASE 1 ONE. Every proof so far called one
// capability at a time, which is the scenario where the mechanism pays and never
// charges. The project's own rule after the F86 pool says to measure where it
// charges — and for a shared global, that is two callers.
//
// THE DETECTOR IS THE ANSWER'S OWN CONTENT: OriginOf is asked about messages in
// TWO DIFFERENT conversations, so a crossed answer names the wrong chat. Nothing
// about timing is asserted; only whether an answer belongs to its question.
func TestProbeParkedStateUnderConcurrency(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CONC") == "" {
		t.Skip("set WA_PROBE_CONC=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
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

	// DUAS MENSAGENS DE CONVERSAS DIFERENTES. O chat de cada uma e' a etiqueta
	// que denuncia a troca.
	var raw string
	const pick = `(() => {
		try {
			const ms = window.require("WAWebCollections").Msg.getModelsArray();
			const byChat = {};
			for (const m of ms) {
				const r = m.id && m.id.remote;
				const s = (r && r._serialized) ? r._serialized : r;
				if (!s || !m.id || !m.id.id) { continue; }
				if (!byChat[s]) { byChat[s] = m.id.id; }
			}
			const chats = Object.keys(byChat);
			if (chats.length < 2) { return JSON.stringify({}); }
			return JSON.stringify({
				chatA: chats[0], msgA: byChat[chats[0]],
				chatB: chats[1], msgB: byChat[chats[1]]});
		} catch (e) { return JSON.stringify({}); }
	})()`
	if err := eval(ctx, pick, &raw); err != nil {
		t.Fatalf("picking messages: %v", err)
	}
	var pickd struct{ ChatA, MsgA, ChatB, MsgB string }
	if err := json.Unmarshal([]byte(raw), &pickd); err != nil {
		t.Fatalf("pick payload: %v", err)
	}
	if pickd.MsgA == "" || pickd.MsgB == "" {
		t.Skip("this session does not hold messages from two different chats")
	}

	r := message.New(runner, eval)
	// AMOSTRA MAIOR DEPOIS DA CORRECAO: doze bastaram para expor o defeito (12 de
	// 12 cruzaram), mas afirmar a AUSENCIA dele e mais caro que expor a presenca.
	const rounds = 40
	var mu sync.Mutex
	crossed, failed := 0, 0
	var firstCross string

	for i := 0; i < rounds; i++ {
		var wg sync.WaitGroup
		wg.Add(2)
		check := func(msg, wantChat, who string) {
			defer wg.Done()
			got, err := r.OriginOf(ctx, msg, "probe/conc/"+who+strconv.Itoa(i))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			if got.ChatJID != wantChat {
				crossed++
				if firstCross == "" {
					firstCross = who + " asked about its own chat and got another's"
				}
			}
		}
		go check(pickd.MsgA, pickd.ChatA, "A")
		go check(pickd.MsgB, pickd.ChatB, "B")
		wg.Wait()
	}

	t.Logf("%d rodadas concorrentes: %d respostas TROCADAS, %d erros", rounds, crossed, failed)
	if crossed > 0 {
		t.Fatalf("CROSSED ANSWERS: %d of %d concurrent reads returned another "+
			"call's answer (%s). Every reader in this capability parks on one page "+
			"global, so two callers on one session overwrite each other — and the "+
			"wrong answer is well formed, which is why nothing caught it",
			crossed, rounds*2, firstCross)
	}
	t.Log("nenhuma resposta trocada nesta amostra")
}
