package waheadless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/chatstate"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeChatChangedFields measures whether the page says WHICH field of a
// chat moved, which is the single cause behind several PARTIAL event rows.
//
// CHAT_ARCHIVED, UNREAD_COUNT and their neighbours all carry the same note: our
// chat.changed is coarse — it says the conversation moved, not what about it
// (H87). The reference has a distinct event per field. Before designing anything
// this asks the only question that matters: does the collection hand the changed
// attribute names to the handler at all?
//
// If it does, the classification belongs in Go (invariant 6), exactly like the
// gp2 subtypes in H119: the page carries the word, Go decides what it means.
//
// READ ONLY apart from one message to the lab peer, which is what makes an
// unread count and a timestamp move so there is something to observe.
func TestProbeChatChangedFields(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHATFIELDS") == "" {
		t.Skip("set WA_PROBE_CHATFIELDS=1 (sends one message to the lab peer)")
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

	// O OUVINTE E' INSTALADO CRU, sem passar pelo ingress: a pergunta e' o que a
	// PAGINA oferece, e medir atraves do nosso barramento mediria o nosso
	// barramento, que ja se sabe que joga fora essa informacao.
	const install = `(() => {
		window.__cf = {seen: [], installed: false, err: ''};
		try {
			const CC = window.require("WAWebChatCollection").ChatCollection;
			window.__cfHandler = function (c) {
				try {
					const rec = {};
					// NAO E' BACKBONE: c.changed nao existe neste build (medido,
					// 0 de 40 eventos). A contabilidade propria vive em __changes
					// e __fired, e e' isso que se le aqui — ler o nome que a
					// referencia usaria daria zero para sempre, que foi exatamente
					// o que a primeira versao desta sonda produziu.
					const pick = v => {
						if (!v) { return null; }
						if (Array.isArray(v)) { return v.slice(0, 12).map(String); }
						if (typeof v === "object") { return Object.keys(v).sort().slice(0, 12); }
						return [String(v).slice(0, 40)];
					};
					rec.changed = pick(c && c.__changes);
					rec.fired = pick(c && c.__fired);
					rec.argKeys = (c && typeof c === "object") ? Object.keys(c).filter(k => k.indexOf("__x_") !== 0).slice(0, 10) : [];
					rec.phase = window.__cfPhase || "?";
					if (window.__cf.seen.length < 300) { window.__cf.seen.push(rec); }
				} catch (e) {}
			};
			CC.on("change", window.__cfHandler);
			window.__cf.installed = true;
		} catch (e) { window.__cf.err = String(e).slice(0, 140); }
		return "ok";
	})()`
	var ok string
	if err := eval(ctx, install, &ok); err != nil {
		t.Fatalf("install: %v", err)
	}
	// CADA ATO GANHA UM ROTULO. Sem isso a contagem final e' uma lista de nomes
	// sem dono, e a pergunta desta sonda e' precisamente de quem e' cada nome.
	phase := func(name string) {
		var s string
		if err := eval(ctx, `(() => { window.__cfPhase = `+strconv.Quote(name)+`; return "ok"; })()`, &s); err != nil {
			t.Fatalf("phase %s: %v", name, err)
		}
	}

	// TRES ATOS DIFERENTES, para que os nomes que aparecem possam ser atribuidos.
	// Uma so' acao produziria uma lista de campos sem como saber qual acao a
	// causou — e a pergunta aqui e' exatamente essa correspondencia.
	phase("send")
	if _, err := send.Text(ctx, runner, eval, peer, "wa-headless chat-field probe", "probe/chatfields"); err != nil {
		t.Fatalf("send: %v", err)
	}
	time.Sleep(6 * time.Second)

	cs := chatstate.New(runner, eval)
	phase("archive")
	if _, err := cs.SetArchived(ctx, peer, true, "probe/chatfields/archive"); err != nil {
		t.Logf("SetArchived(true): %v", err)
	}
	time.Sleep(4 * time.Second)
	// O RESTAURO E' IMEDIATO. Arquivar o chat do par estragaria o fixture de
	// todo teste que o procura na lista ativa.
	phase("unarchive")
	if _, err := cs.SetArchived(ctx, peer, false, "probe/chatfields/unarchive"); err != nil {
		t.Errorf("RESTORE FAILED, the lab chat is still archived: %v", err)
	}
	time.Sleep(4 * time.Second)

	phase("unread")
	// AS DUAS IDENTIDADES. MarkUnread recusou o jid de telefone na rodada
	// anterior com "no such conversation", que e' o cheiro exato da H148.
	ident, ierr := lookup.New(runner, eval).NumberID(ctx, peer, "probe/chatfields/resolve")
	if ierr != nil {
		t.Fatalf("resolving the peer: %v", ierr)
	}
	ch := chats.New(runner, eval)
	if _, err := ch.MarkUnread(ctx, peer, "probe/chatfields/unread-asked"); err != nil {
		t.Logf("MarkUnread(asked-for jid): %v", err)
	}
	if _, err := ch.MarkUnread(ctx, ident.JID, "probe/chatfields/unread-resolved"); err != nil {
		t.Logf("MarkUnread(resolved jid): %v", err)
	}
	time.Sleep(6 * time.Second)

	var raw string
	const read = `(() => {
		const s = window.__cf;
		if (!s) { return JSON.stringify({err: "state missing"}); }
		const byPhase = {};
		for (const r of s.seen) {
			const p = r.phase || "?";
			byPhase[p] = byPhase[p] || {events: 0, fired: {}};
			byPhase[p].events++;
			if (r.fired) { for (const k of r.fired) { byPhase[p].fired[k] = (byPhase[p].fired[k] || 0) + 1; } }
		}
		return JSON.stringify({installed: s.installed, err: s.err,
			events: s.seen.length, byPhase: byPhase});
	})()`
	if err := eval(ctx, read, &raw); err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Logf("chat change fields: %s", raw)

	var cleanup string
	_ = eval(ctx, `(() => { try { window.require("WAWebChatCollection").ChatCollection.off("change", window.__cfHandler); } catch (e) {} return "ok"; })()`, &cleanup)
}
