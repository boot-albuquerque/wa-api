package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/channel"
	"wa-api/internal/wa-headless/capabilities/lookup"
)

// TestProbeChannelAdminChain measures the WHOLE admin chain before any of it is
// turned into a capability.
//
// Five ledger rows depend on it — send, accept, revoke, demote, transfer — and
// they are SEQUENTIAL: nothing after step two can be measured if step two fails.
// Writing five capabilities first and discovering that would be five wasted
// pieces of work, so the raw calls are exercised in order and each step reports
// what it did.
//
// It needs BOTH accounts awake: conta-A owns the channel and invites, conta-B
// accepts. The channel is created for this and deleted at the end.
func TestProbeChannelAdminChain(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CHANADMIN") == "" {
		t.Skip("set WA_PROBE_CHANADMIN=1 (creates a channel and invites conta-B as admin)")
	}
	profileA := os.Getenv("WA_SEND_FROM_PROFILE")
	profileB := os.Getenv("WA_SEND_TO_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profileA == "" || profileB == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE and WA_SEND_TO_JID are required")
	}

	d, ctx, closeAll := openDual(t, profileA, profileB, 10*time.Minute)
	defer closeAll()
	evalA := d.A.Tab().Evaluate
	evalB := d.B.Tab().Evaluate

	// conta-A creates the channel. Deletion is registered immediately.
	mA := channel.NewManager(d.RunnerA, evalA)
	made, err := mA.Create(ctx, "wa-headless admin chain "+strconv.FormatInt(time.Now().Unix(), 10),
		"", "probe/chanadmin")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Logf("channel created: %s", made)
	defer func() {
		if err := mA.Delete(ctx, made.JID, made.InviteCode, "probe/chanadmin-undo"); err != nil {
			t.Errorf("DELETE FAILED: a real channel is left standing: %v", err)
		} else {
			t.Log("channel deleted")
		}
	}()

	park := func(t *testing.T, eval func(context.Context, string, *string) error,
		key, script, what string) map[string]any {
		t.Helper()
		var ignored string
		if err := eval(ctx, script, &ignored); err != nil {
			t.Fatalf("%s kick: %v", what, err)
		}
		deadline := time.Now().Add(45 * time.Second)
		for {
			var raw string
			if err := eval(ctx, "window."+key, &raw); err != nil {
				t.Fatalf("%s read: %v", what, err)
			}
			if raw != "" && raw != "null" {
				var v map[string]any
				if err := json.Unmarshal([]byte(raw), &v); err != nil {
					t.Fatalf("%s payload: %v", what, err)
				}
				return v
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s never answered", what)
			}
			time.Sleep(400 * time.Millisecond)
		}
	}

	// THE PEER'S RESOLVED IDENTITY, not the phone jid a human typed.
	//
	// The first attempt scanned for the phone form and found nothing: this build
	// files chats under a LID, and comparing the caller's jid is the H34 mistake
	// in a new place. lookup.NumberID is the module's own resolution, proven
	// today, so it is used rather than a second one written here.
	res := lookup.New(d.RunnerA, evalA)
	ident, err := res.NumberID(ctx, peer, "probe/chanadmin")
	if err != nil {
		t.Fatalf("resolving the invitee: %v", err)
	}
	t.Logf("invitee resolved: %s", ident)
	peerResolved := ident.JID

	// STEP 1 — conta-A sends the admin invite to conta-B.
	sendScript := `(() => {
		window.__ca1 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const W = window.require("WAWebWidFactory");
			const C = window.require("WAWebCollections");
			const chanWid = W.createWid(` + strconv.Quote(made.JID) + `);
			const userWid = W.createWid(` + strconv.Quote(peerResolved) + `);
			// O find da colecao esta QUEBRADO neste build (findImpl ausente, o
			// mesmo defeito da H123), entao o chat tem de ser achado sem ele.
			// Ele existe: esta sessao trocou mensagens com o par hoje.
			let chat = null;
			for (const cand of [userWid, ` + strconv.Quote(peer) + `]) {
				try { chat = C.Chat.get(cand); } catch (e) {}
				if (chat) { break; }
			}
			if (!chat) {
				// Varredura, que e a busca honesta quando o indice nao coopera.
				const all = typeof C.Chat.getModelsArray === "function" ? C.Chat.getModelsArray() : [];
				for (const c of all) {
					const id = c.id;
					const sid = (id && id._serialized) ? id._serialized : "";
					if (sid === ` + strconv.Quote(peerResolved) + ` || (id && id.user === userWid.user)) { chat = c; break; }
				}
			}
			if (!chat) { window.__ca1 = JSON.stringify({ok:false, why:"no chat with the invitee"}); return; }
			const r = await window.require("WAWebNewsletterSendMsgAction")
				.sendNewsletterAdminInviteMessage(chat, {
					newsletterWid: chanWid, invitee: userWid,
					inviteMessage: "wa-headless probe", base64Thumb: null,
				});
			window.__ca1 = JSON.stringify({ok:true, result: (r && r.messageSendResult) || String(r)});
		} catch (e) { window.__ca1 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	step1 := park(t, evalA, "__ca1", sendScript, "send-invite")
	t.Logf("STEP 1 send invite (A): %v", step1)
	if ok, _ := step1["ok"].(bool); !ok {
		t.Logf("MEASURED: the chain stops at step 1; the four steps after it cannot " +
			"be measured, and saying anything about them would be invention")
		return
	}

	// STEP 2 — conta-B accepts.
	acceptScript := `(() => {
		window.__ca2 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			await window.require("WAWebMexAcceptNewsletterAdminInviteJob")
				.acceptNewsletterAdminInvite(` + strconv.Quote(made.JID) + `);
			window.__ca2 = JSON.stringify({ok:true});
		} catch (e) { window.__ca2 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	// STEP 1b — REVOGAR ANTES DE ACEITAR, que é a ordem em que revogar faz
	// sentido. A primeira execução testou depois do aceite e o servidor
	// respondeu "Not Allowed" — resposta correta para um convite já consumido,
	// e medição errada da capacidade.
	revokeFirstScript := `(() => {
		window.__ca1b = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const W = window.require("WAWebWidFactory");
			await window.require("WAWebMexRevokeNewsletterAdminInviteJob")
				.revokeNewsletterAdminInvite(
					W.createWid(` + strconv.Quote(made.JID) + `),
					W.createWid(` + strconv.Quote(peerResolved) + `));
			window.__ca1b = JSON.stringify({ok:true});
		} catch (e) { window.__ca1b = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	// A ORDEM É UMA CHAVE, porque as duas ordens provam coisas DIFERENTES e uma
	// execução só não prova as duas:
	//
	//   sem a chave  — convite ACEITO: prova `accept` (assinantes 0 -> 1).
	//   com a chave  — convite REVOGADO antes: prova `revoke`, porque o aceite
	//                  seguinte falha com "Not Found" e os assinantes ficam em 0.
	//
	// Medidas nas duas ordens; a nota do ledger cita as duas.
	revokeFirst := os.Getenv("WA_PROBE_CHANADMIN_REVOKE_FIRST") != ""
	if revokeFirst {
		t.Logf("STEP 1b revoke BEFORE accept (A): %v",
			park(t, evalA, "__ca1b", revokeFirstScript, "revoke-first"))
	} else {
		t.Log("STEP 1b skipped: set WA_PROBE_CHANADMIN_REVOKE_FIRST=1 to prove revoke")
	}

	// STEP 2 — conta-B tenta aceitar um convite que acabou de ser revogado.
	// Se a revogação funcionou, isto tem de FALHAR — e é essa falha que prova a
	// revogação, não o silêncio da chamada anterior.
	step2 := park(t, evalB, "__ca2", acceptScript, "accept-invite")
	t.Logf("STEP 2 accept (B): %v", step2)
	accepted, _ := step2["ok"].(bool)
	if revokeFirst && accepted {
		t.Error("the accept SUCCEEDED after a revoke; the revoke did not take, and " +
			"the call returning ok is exactly the silent success this probe exists to catch")
	}
	if !revokeFirst && !accepted {
		t.Errorf("the accept failed on an invite nobody revoked: %v", step2["why"])
	}

	// STEP 3 — conta-A demotes conta-B.
	//
	// THE ACTION WANTS A MODEL, NOT A WID. The first attempt passed a Wid and got
	// "Data passed to getter must include an id property", which is the same
	// answer the subscription path gave in H123 — this build's actions take the
	// memoised collection model. conta-A OWNS the channel, so the model is in its
	// own newsletter collection and no broken find is needed.
	demoteScript := `(() => {
		window.__ca3 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const W = window.require("WAWebWidFactory");
			const NC = window.require("WAWebCollections").WAWebNewsletterCollection;
			const ch = NC.get(` + strconv.Quote(made.JID) + `);
			if (!ch) { window.__ca3 = JSON.stringify({ok:false, why:"own channel not in the collection"}); return; }
			await window.require("WAWebDemoteNewsletterAdminAction")
				.demoteNewsletterAdminAction(ch, [W.createWid(` + strconv.Quote(peerResolved) + `)]);
			window.__ca3 = JSON.stringify({ok:true});
		} catch (e) { window.__ca3 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	step3 := park(t, evalA, "__ca3", demoteScript, "demote")
	t.Logf("STEP 3 demote (A): %v", step3)

	// STEP 4 — revoke an admin invite. Measured on the same channel; if the
	// demote already removed the admin, the revoke has nothing to revoke and the
	// page's own answer says which.
	revokeScript := `(() => {
		window.__ca4 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const W = window.require("WAWebWidFactory");
			await window.require("WAWebMexRevokeNewsletterAdminInviteJob")
				.revokeNewsletterAdminInvite(
					W.createWid(` + strconv.Quote(made.JID) + `),
					W.createWid(` + strconv.Quote(peerResolved) + `));
			window.__ca4 = JSON.stringify({ok:true});
		} catch (e) { window.__ca4 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	t.Logf("STEP 4 revoke (A): %v", park(t, evalA, "__ca4", revokeScript, "revoke"))

	// STEP 5 — transfer ownership. MEASURED ONLY, and deliberately last: if it
	// succeeded, conta-A would stop owning the channel and could not delete it,
	// so the delete registered at the top would fail and leave a real channel
	// standing. That is the reason this step exists at the end and reports rather
	// than being retried.
	transferScript := `(() => {
		window.__ca5 = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const M = window.require("WAWebChangeNewsletterOwnerAction");
			window.__ca5 = JSON.stringify({ok:true, keys: Object.keys(M).slice(0,8)});
		} catch (e) { window.__ca5 = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	t.Logf("STEP 5 transfer surface (A, NOT executed): %v",
		park(t, evalA, "__ca5", transferScript, "transfer-surface"))

	// The reader's own view, which is the only postcondition available here.
	r := channel.New(d.RunnerA, evalA)
	back, err := r.ByInviteCode(ctx, made.InviteCode, "probe/chanadmin")
	if err != nil {
		t.Errorf("read back: %v", err)
	} else {
		t.Logf("channel after the chain: %s", back)
		// OS ASSINANTES SÃO A PÓS-CONDIÇÃO INDEPENDENTE. Um aceite que funcionou
		// leva o canal de 0 para 1; um revogado deixa em 0. É a diferença entre
		// "a chamada não lançou" e "a coisa aconteceu".
		if revokeFirst && back.Subscribers != 0 {
			t.Errorf("after a revoke the channel has %d subscriber(s); the revoke did "+
				"not stop the join", back.Subscribers)
		}
		if !revokeFirst && back.Subscribers == 0 {
			t.Error("after an accepted admin invite the channel still has no " +
				"subscribers; the accept did not take")
		}
	}
}
