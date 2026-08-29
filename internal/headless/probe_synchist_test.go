package headless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/fetchmessages"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeSyncHistorySurface measures two rows whose notes describe an absence
// that was never re-checked with the enumerate-then-read-the-signature technique
// that has paid four times today.
//
// syncHistory is PARTIAL on "buscamos histórico de uma conversa; sincronizar
// não". The reference does something specific and small
// (wwebjs_client.js:3173-3189): guard on chat.endOfHistoryTransferType === 0, then
// WAWebSendNonMessageDataRequest.sendPeerDataOperationRequest(3, {chatId}). That
// asks the PHONE to send history — a different act from reading the local store,
// which is what our fetchmessages does. The note is right about the difference
// and silent about whether the module is here.
//
// reject (call) is PARTIAL on H130 having measured four call-action modules
// ABSENT. Four names is a good sample and not an exhaustive one, and this week
// three invented names were fixed by enumerating instead of guessing.
//
// READ ONLY: nothing is sent, no history is requested, no call is answered.
func TestProbeSyncHistorySurface(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SYNCHIST") == "" {
		t.Skip("set WA_PROBE_SYNCHIST=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	var raw string
	const script = `(() => {
		const out = {};
		const safe = e => String((e && e.message) || e).slice(0,110);
		// 1) O MODULO DA REFERENCIA PARA syncHistory.
		try {
			const M = window.require("WAWebSendNonMessageDataRequest");
			out.syncModule = M ? Object.keys(M).slice(0, 10) : "empty";
			if (M && typeof M.sendPeerDataOperationRequest === "function") {
				out.sendPeerArity = M.sendPeerDataOperationRequest.length;
				out.sendPeerSrc = String(M.sendPeerDataOperationRequest).slice(0, 220);
			}
		} catch (e) { out.syncModule = "absent (" + safe(e) + ")"; }

		// 2) A GUARDA. endOfHistoryTransferType === 0 e' o que decide se ha o que
		//    pedir; sem ela, "a chamada nao falhou" nao diz nada.
		try {
			const CC = window.require("WAWebChatCollection").ChatCollection;
			const all = CC.getModelsArray();
			const tally = {};
			let withField = 0;
			for (const c of all) {
				const v = c.endOfHistoryTransferType;
				if (typeof v === "undefined") { continue; }
				withField++;
				tally[String(v)] = (tally[String(v)] || 0) + 1;
			}
			out.chats = all.length;
			out.withEndOfHistoryField = withField;
			out.endOfHistoryValues = tally;
		} catch (e) { out.guardErr = safe(e); }

		// 3) A SUPERFICIE DE RECUSA DE CHAMADA, enumerada em vez de listada por
		//    nome: a H130 mediu quatro nomes ausentes, que e' amostra e nao censo.
		const callish = [];
		for (const m of ["WAWebCallCollection","WAWebApiCall","WAWebCallActions",
		                  "WAWebRejectCallAction","WAWebEndCallAction","WAWebOfferCallAction",
		                  "WAWebCallSignaling","WAWebCallModel","WAWebCallState"]) {
			try { const mod = window.require(m);
				callish.push({m: m, keys: mod ? Object.keys(mod).slice(0, 8) : []});
			} catch (e) { callish.push({m: m, keys: "absent"}); }
		}
		out.callModules = callish;
		try {
			const C = window.require("WAWebCollections");
			const names = [];
			for (const k of Object.keys(C)) { if (/call/i.test(k)) { names.push(k); } }
			out.callCollections = names;
		} catch (e) {}
		return JSON.stringify(out);
	})()`
	if err := sess.Tab().Evaluate(ctx, script, &raw); err != nil {
		t.Fatalf("eval: %v", err)
	}
	t.Logf("surface: %s", raw)

	// AS DUAS RESPOSTAS. Uma capacidade so' vista dizendo "sim" nao esta provada
	// — a licao da H147 —, e aqui as duas existem no mesmo roster: 378 chats com
	// transferType 0 (ha' o que pedir) e 5 com valor diferente (nao ha').
	f := fetchmessages.New(runner, sess.Tab().Evaluate)
	var eligible, done string
	const pick = `(() => {
		try {
			const CC = window.require("WAWebChatCollection").ChatCollection;
			let a = "", b = "";
			for (const c of CC.getModelsArray()) {
				const t = c.endOfHistoryTransferType;
				const s = (c.id && c.id._serialized) ? c.id._serialized : "";
				if (!s) { continue; }
				if (t === 0 && !a) { a = s; }
				if (typeof t === "number" && t !== 0 && !b) { b = s; }
				if (a && b) { break; }
			}
			return JSON.stringify({eligible: a, done: b});
		} catch (e) { return JSON.stringify({}); }
	})()`
	var picked string
	if err := sess.Tab().Evaluate(ctx, pick, &picked); err != nil {
		t.Fatalf("picking chats: %v", err)
	}
	var sel struct{ Eligible, Done string }
	if err := json.Unmarshal([]byte(picked), &sel); err != nil {
		t.Fatalf("picked payload: %v", err)
	}
	eligible, done = sel.Eligible, sel.Done

	if eligible == "" {
		t.Fatal("no chat with transferType 0; the request path cannot be exercised")
	}
	yes, err := f.SyncHistory(ctx, eligible, "probe/synchist/yes")
	if err != nil {
		t.Fatalf("SyncHistory on an eligible chat: %v", err)
	}
	t.Logf("eligible chat: %s", yes)
	if !yes.Requested {
		t.Fatal("a chat with transferType 0 did not produce a request")
	}

	if done == "" {
		t.Log("no chat past its transfer in this session; the NO path stays unexercised")
		return
	}
	no, err := f.SyncHistory(ctx, done, "probe/synchist/no")
	if err != nil {
		t.Fatalf("SyncHistory on a completed chat: %v", err)
	}
	t.Logf("completed chat: %s", no)
	if no.Requested {
		t.Fatal("a chat past its transfer still produced a request; the guard is not guarding")
	}
	t.Log("PROVEN: both answers — it asks when there is something to ask for, and refuses when there is not")
}
