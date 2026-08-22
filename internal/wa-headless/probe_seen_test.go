package waheadless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/capabilities/send"
)

// TestProbeSeenAcrossSessions proves sendSeen by the fact it exists to produce,
// instead of by the counter that H82 correctly refused to trust.
//
// H82 demoted this row for a good reason: MarkRead's postcondition asserts that
// chat.unreadCount moved IN THE SAME SESSION, and H78 measured that counter as
// cross-session. H52 proved it against one chat where it did move; whether that
// generalises stayed open, and asserting a counter that may not move is how a
// capability reports success it cannot see.
//
// THE COUNTER IS THE WRONG WITNESS ANYWAY. What sendSeen does that anybody can
// observe is tell the SENDER their message was read — ack 3 on their copy. That
// is a fact about the other session, which is exactly what no single session
// could check, and exactly what the dual harness (H135) is for.
//
// conta-B sends, conta-A marks read, and conta-B's own copy is watched. The
// baseline matters: a message already at ack 3 before marking would prove
// nothing, so this refuses to start from there.
func TestProbeSeenAcrossSessions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SEEN") == "" {
		t.Skip("set WA_PROBE_SEEN=1 (one message between the lab accounts)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	selfA := os.Getenv("WA_SELF_A_JID")
	if pa == "" || pb == "" || selfA == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B and WA_SELF_A_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 10*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate

	// conta-B manda para conta-A.
	sent, err := send.Text(ctx, d.RunnerB, evalB, selfA, "wa-headless seen probe", "probe/seen")
	if err != nil {
		t.Fatalf("B->A: %v", err)
	}
	t.Logf("conta-B sent: %s", sent)

	ackOnB := func(what string) int {
		t.Helper()
		var raw string
		script := `(() => {
			try {
				for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
					if (m.id && m.id.id === ` + strconv.Quote(sent.ID.ID) + `) {
						return String(typeof m.ack === "number" ? m.ack : -1);
					}
				}
				return "-2";
			} catch (e) { return "-3"; }
		})()`
		if err := evalB(ctx, script, &raw); err != nil {
			t.Fatalf("ack read (%s): %v", what, err)
		}
		n, _ := strconv.Atoi(raw)
		return n
	}

	// O CONFUNDIDOR TEM DE CAIR ANTES DA CONCLUSAO. Se conta-A tiver recibo de
	// leitura DESLIGADO, o ack nunca chega a 3 por decisao de privacidade, e
	// atribuir isso ao MarkRead seria diagnostico errado com aparencia de
	// medicao.
	var receipts string
	const readReceiptScript = `(() => {
		const out = {};
		const safe = e => String((e && e.message) || e).slice(0,110);
		for (const m of ["WAWebUserPrefsGeneral","WAWebPrivacyPreferences",
		                  "WAWebUserPrefsPrivacy","WAWebReadReceiptsPrefs"]) {
			try { const mod = window.require(m);
				out[m] = mod ? Object.keys(mod).filter(k => /receipt|read/i.test(k)).slice(0,8) : "empty";
			} catch (e) { out[m] = "absent"; }
		}
		try {
			const P = window.require("WAWebUserPrefsGeneral");
			for (const k of Object.keys(P)) {
				if (!/receipt/i.test(k)) { continue; }
				try { const v = P[k]; out["value_" + k] = (typeof v === "function") ? String(v()) : String(v); }
				catch (e) { out["value_" + k] = "threw: " + safe(e); }
			}
		} catch (e) { out.prefsErr = safe(e); }
		return JSON.stringify(out);
	})()`
	if err := evalA(ctx, readReceiptScript, &receipts); err != nil {
		t.Fatalf("read-receipt setting: %v", err)
	}
	t.Logf("conta-A read-receipt surface: %s", receipts)

	time.Sleep(5 * time.Second)
	before := ackOnB("before")
	t.Logf("ack on conta-B BEFORE conta-A reads: %d", before)
	if before < 0 {
		t.Fatalf("conta-B cannot see its own message (ack %d); nothing below means anything", before)
	}
	if before >= 3 {
		t.Skip("the message was already read before conta-A acted; a baseline of 3 " +
			"cannot show a transition, and waiting for one would be waiting for nothing")
	}

	// conta-A marca lida. A IDENTIDADE E' A RESOLVIDA — a lição da H148, e o
	// MarkUnread deste mesmo pacote recusa o jid de telefone.
	ident, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, os.Getenv("WA_PEER_B_JID"), "probe/seen/resolve")
	if err != nil {
		t.Fatalf("resolving conta-B from conta-A: %v", err)
	}
	// O ERRO NAO ENCERRA A MEDICAO, e essa e' a pergunta desta sonda. A
	// pos-condicao do MarkRead afirma que `unreadCount` mudou NA MESMA SESSAO, e
	// a H78/H82 mediram esse contador como CROSS_SESSION — entao a falha e'
	// compativel com "nao fez nada" E com "fez, e o contador local nao conta".
	// Quem separa as duas e' o OUTRO lado.
	res, err := chats.New(d.RunnerA, evalA).MarkRead(ctx, ident.JID, "probe/seen/markread")
	t.Logf("conta-A MarkRead: %s err=%v", res, err)
	markFailed := err != nil

	deadline := time.Now().Add(60 * time.Second)
	for {
		got := ackOnB("after")
		if got >= 3 {
			if markFailed {
				t.Logf("MEASURED: the read receipt DID leave (ack %d, was %d) while "+
					"MarkRead reported failure. The act works and the postcondition "+
					"is watching the wrong counter — which is exactly the open "+
					"question H82 left.", got, before)
			} else {
				t.Logf("PROVEN: conta-B's message reached ack %d after conta-A read "+
					"it (was %d)", got, before)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Logf("conta-A marked the chat read (err=%v) and conta-B's copy stayed "+
				"at ack %d (was %d)", err, got, before)
			// O DISCRIMINADOR. "O recibo nao saiu" e "esta conta tem recibo
			// desligado" produzem o MESMO ack, e os nomes de modulo de privacidade
			// que chutei nao existem neste build. O caminho inverso separa os dois
			// sem precisar achar a configuracao: se conta-B marcar lida e o ack de
			// conta-A subir, o mecanismo funciona e a diferenca e' da conta; se
			// falhar dos dois lados, e' o MarkRead.
			reverseSeen(t, d, ctx)
			return
		}
		time.Sleep(2 * time.Second)
	}
}

func reverseSeen(t *testing.T, d *dualSession, ctx context.Context) {
	t.Helper()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate
	peerB := os.Getenv("WA_PEER_B_JID")
	selfA := os.Getenv("WA_SELF_A_JID")

	sent, err := send.Text(ctx, d.RunnerA, evalA, peerB, "wa-headless reverse seen probe", "probe/seen/rev")
	if err != nil {
		t.Logf("REVERSE: conta-A could not send: %v", err)
		return
	}
	ackOnA := func() int {
		var raw string
		script := `(() => {
			try {
				for (const m of window.require("WAWebCollections").Msg.getModelsArray()) {
					if (m.id && m.id.id === ` + strconv.Quote(sent.ID.ID) + `) {
						return String(typeof m.ack === "number" ? m.ack : -1);
					}
				}
				return "-2";
			} catch (e) { return "-3"; }
		})()`
		if err := evalA(ctx, script, &raw); err != nil {
			return -4
		}
		n, _ := strconv.Atoi(raw)
		return n
	}
	time.Sleep(5 * time.Second)
	before := ackOnA()
	t.Logf("REVERSE: ack on conta-A before conta-B reads: %d", before)
	if before >= 3 {
		t.Log("REVERSE: already read; this leg cannot discriminate")
		return
	}
	identA, err := lookup.New(d.RunnerB, evalB).NumberID(ctx, selfA, "probe/seen/rev-resolve")
	if err != nil {
		t.Logf("REVERSE: conta-B could not resolve conta-A: %v", err)
		return
	}
	res, err := chats.New(d.RunnerB, evalB).MarkRead(ctx, identA.JID, "probe/seen/rev-markread")
	t.Logf("REVERSE: conta-B MarkRead: %s err=%v", res, err)

	deadline := time.Now().Add(45 * time.Second)
	for {
		got := ackOnA()
		if got >= 3 {
			t.Logf("REVERSE: ack reached %d — the mechanism WORKS and the first leg's "+
				"failure is about conta-A, not about MarkRead", got)
			return
		}
		if time.Now().After(deadline) {
			t.Logf("REVERSE: ack stayed at %d — it fails in BOTH directions, so the "+
				"read receipt does not leave and the row is about the capability, "+
				"not about one account's privacy setting", got)
			return
		}
		time.Sleep(2 * time.Second)
	}
}
