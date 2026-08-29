package headless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeMarkUnreadPrimitives follows the lead H160 registered and did not
// chase: WAWebUpdateUnreadChatAction exports markUnread, and markChatUnread is
// BLOCKED since H78 on "duas primitivas medidas, nenhuma marca".
//
// A THIRD PRIMITIVE IS NOT A REASON TO REOPEN A BLOCKED ROW BY ITSELF — H160 is.
// There, MarkRead's postcondition failed for two days because we called
// sendConversationSeen where the reference calls UpdateUnreadChatAction.sendSeen,
// and swapping the module made the counter move. The same module, one export
// over, is the most specific lead this row has ever had.
//
// EACH PRIMITIVE IS TRIED SEPARATELY AND MEASURED SEPARATELY, because H150 spent
// a round on an instrument that lumped three acts into one tally and would have
// confirmed the wrong hypothesis. The chat is restored to read afterwards.
func TestProbeMarkUnreadPrimitives(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MARKUNREAD2") == "" {
		t.Skip("set WA_PROBE_MARKUNREAD2=1 (marks a lab chat unread, then restores it)")
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
	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/mu2/resolve")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}

	script := `(() => {
		window.__mu = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,130);
		(async () => {
		const out = {tries: []};
		const CC = window.require("WAWebChatCollection").ChatCollection;
		const key = ` + strconv.Quote(ident.JID) + `;
		// A COLECAO E' RELIDA A CADA LEITURA. O primeiro instrumento guardou o
		// modelo numa variavel e leu dele; depois da primeira chamada o campo
		// virou nao-numerico, porque markUnread passa o chat por
		// WAWebStateUtils.unproxy e o objeto em maos deixa de ser o vivo. E' a
		// lição da H143 aplicada a um caso novo — e ela ja custou uma rodada aqui.
		const live = () => CC.get(key);
		const chat = live();
		const state = () => {
			const c = live();
			return {
				markedUnread: !!(c && c.markedUnread),
				unreadCount: (c && typeof c.unreadCount === "number") ? c.unreadCount : -1,
				stillInCollection: !!c
			};
		};
		try {
			if (!chat) { out.err = "chat not in collection"; window.__mu = JSON.stringify(out); return; }
			out.initial = state();

			// A CONTRADICAO E' RESOLVIDA POR MEDICAO. A H160 enumerou este modulo
			// e listou markUnread entre as chaves; a chamada acusou o modulo
			// indefinido. Uma das duas leituras esta errada e a resposta nao sai
			// de raciocinio.
			try {
				const A = window.require("WAWebUpdateUnreadChatAction");
				out.moduleType = typeof A;
				out.moduleKeys = A ? Object.keys(A).sort() : null;
				out.markUnreadType = A ? typeof A.markUnread : "n/a";
				out.markUnreadArity = (A && typeof A.markUnread === "function") ? A.markUnread.length : -1;
				out.sendSeenType = A ? typeof A.sendSeen : "n/a";
				// A ASSINATURA REAL, lida da funcao. Chutar a forma de uma
				// funcao de aridade 3 e' como os quatro nomes inventados desta
				// semana: barato de evitar, caro de errar.
				if (A && typeof A.markUnread === "function") {
					out.markUnreadSrc = String(A.markUnread).slice(0, 700);
				}
				if (A && typeof A.sendSeen === "function") {
					out.sendSeenSrc = String(A.sendSeen).slice(0, 160);
				}
			} catch (e) { out.moduleErr = safe(e); }

			let Stream = null;
			try { Stream = window.require("WAWebStreamModel").Stream; } catch (e) {}

			// 1) O QUE NOS JA CHAMAMOS. A linha de base tem de incluir a nossa,
			//    senao "a nova funciona" nao se distingue de "qualquer uma agora
			//    funciona".
			try {
				window.require("WAWebCmd").Cmd.markChatUnread(chat, true);
				await new Promise(r => setTimeout(r, 1200));
				out.tries.push({name: "Cmd.markChatUnread", ok: true, after: state()});
			} catch (e) { out.tries.push({name: "Cmd.markChatUnread", ok: false, why: safe(e), after: state()}); }

			// 2) A PISTA: mesma familia que consertou o MarkRead.
			try {
				const A = window.require("WAWebUpdateUnreadChatAction");
				// A FORMA VEM DA ASSINATURA LIDA, nao de chute:
				//   markUnread(chat, unread, allowAction = true)
				await A.markUnread(chat, true);
				await new Promise(r => setTimeout(r, 1200));
				out.tries.push({name: "UpdateUnreadChatAction.markUnread(chat,true)", ok: true, after: state()});
			} catch (e) { out.tries.push({name: "UpdateUnreadChatAction.markUnread(chat,true)", ok: false, why: safe(e), after: state()}); }

			// 3) A MESMA, COM PRESENCA ANUNCIADA. Refutada para o recibo (H160),
			//    mas nunca testada para a marca.
			try {
				const A = window.require("WAWebUpdateUnreadChatAction");
				if (Stream && Stream.markAvailable) { Stream.markAvailable(); }
				try { // A FORMA VEM DA ASSINATURA LIDA, nao de chute:
				//   markUnread(chat, unread, allowAction = true)
				await A.markUnread(chat, true); }
				finally { if (Stream && Stream.markUnavailable) { Stream.markUnavailable(); } }
				await new Promise(r => setTimeout(r, 1200));
				out.tries.push({name: "markUnread bracketed", ok: true, after: state()});
			} catch (e) { out.tries.push({name: "markUnread bracketed", ok: false, why: safe(e), after: state()}); }

			// RESTAURO: a conversa volta a lida, com a primitiva que a H160 provou.
			try {
				if (Stream && Stream.markAvailable) { Stream.markAvailable(); }
				try { await window.require("WAWebUpdateUnreadChatAction").sendSeen({chat: chat, threadId: undefined}); }
				finally { if (Stream && Stream.markUnavailable) { Stream.markUnavailable(); } }
				await new Promise(r => setTimeout(r, 1200));
				out.restored = state();
			} catch (e) { out.restoreErr = safe(e); }
		} catch (e) { out.err = safe(e); }
		window.__mu = JSON.stringify(out);
		})();
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err := eval(ctx, "window.__mu", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the mark-unread probe never answered")
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("mark-unread primitives: %s", raw)
}
