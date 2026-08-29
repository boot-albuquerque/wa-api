package headless

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/capabilities/message"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeProduceMention makes a mention exist so getMentions can be measured.
//
// H106 measured 395 loaded messages with ZERO mentions under five candidate
// field names, and refused to ship a reader nobody had seen return anything.
// That refusal was right and it left the row open on DATA, not on impossibility
// — so the way out is the one that closed the quote (H131), the group events
// (H135), the vote (H121) and the group update (H119): PRODUCE the fact.
//
// It sends ONE message to the lab group mentioning the lab peer. That is an
// outward effect confined to the lab fixture, and the same class this package
// performs routinely.
func TestProbeProduceMention(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MENTION") == "" {
		t.Skip("set WA_PROBE_MENTION=1 (sends a message mentioning the lab peer)")
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

	gjid := findLabGroupJID(ctx, t, runner, eval)
	if gjid == "" {
		t.Skip("lab group not found; a mention needs a group")
	}
	// THE MENTIONED IDENTITY IS THE RESOLVED ONE. Mentioning a phone jid on a
	// LID-first build names somebody the page does not file under that id — the
	// H34 lesson, which cost a run in H136 for the same reason.
	res := lookup.New(runner, eval)
	ident, err := res.NumberID(ctx, peer, "probe/mention")
	if err != nil {
		t.Fatalf("resolving the mentioned peer: %v", err)
	}
	t.Logf("mentioned identity: %s", ident)

	script := `(() => {
		window.__mn = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,150);
		(async () => {
		try {
			const W = window.require("WAWebWidFactory");
			const CC = window.require("WAWebChatCollection").ChatCollection;
			const chat = CC.get(` + strconv.Quote(gjid) + `);
			if (!chat) { window.__mn = JSON.stringify({ok:false, why:"lab group not in the collection"}); return; }
			const wid = W.createWid(` + strconv.Quote(ident.JID) + `);
			// O TERCEIRO ARGUMENTO carrega as opcoes; mentionedJidList quer WIDS,
			// nao strings — a referencia converte antes de chamar.
			// O ASSUNTO DO GRUPO VEM DO PROPRIO MODELO. A referencia exige
			// {groupSubject, groupJid} — o assunto nao e' decorativo, e o que a
			// pagina renderiza no lugar da mencao.
			const subject = String((chat.contact && chat.contact.name) || chat.name || chat.formattedTitle || "grupo");
			await window.require("WAWebSendTextMsgChatAction")
				.sendTextMsgToChat(chat, "wa-headless mention probe", {
					mentionedJidList: [wid],
					groupMentions: [{ groupSubject: subject, groupJid: W.createWid(` + strconv.Quote(gjid) + `) }],
				});
			window.__mn = JSON.stringify({ok:true});
		} catch (e) { window.__mn = JSON.stringify({ok:false, why: safe(e)}); }
		})();
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("send kick: %v", err)
	}
	var raw string
	deadline := time.Now().Add(45 * time.Second)
	for {
		if err := eval(ctx, "window.__mn", &raw); err != nil {
			t.Fatalf("send read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the send never answered")
		}
		time.Sleep(400 * time.Millisecond)
	}
	t.Logf("send with mention: %s", raw)
	if !containsOK(raw) {
		t.Fatalf("the mention could not be produced; nothing after this can be measured")
	}

	// NOW MEASURE WHERE IT LANDED. The reader cannot be written before this: H106
	// searched five candidate names and found none populated, because no message
	// carried a mention at all.
	time.Sleep(3 * time.Second)
	// A PROVA E' LER DE VOLTA PELO CAPABILITY, nao pela varredura que descobriu o
	// campo. A varredura serviu para saber ONDE olhar (H142); se a prova ficasse
	// nela, o que estaria provado era o instrumento.
	const idScript = `(() => {
		window.__mf = null;
		try {
			const ms = window.require("WAWebCollections").Msg.getModelsArray();
			let best = null;
			for (const m of ms) {
				if (!Array.isArray(m.mentionedJidList) || !m.mentionedJidList.length) { continue; }
				if (!best || (m.t || 0) > (best.t || 0)) { best = m; }
			}
			window.__mf = JSON.stringify({found: !!best, id: (best && best.id && best.id.id) || ""});
		} catch (e) { window.__mf = JSON.stringify({err: String(e).slice(0,120)}); }
		return "kicked";
	})()`
	if err := eval(ctx, idScript, &ignored); err != nil {
		t.Fatalf("id kick: %v", err)
	}
	deadline = time.Now().Add(30 * time.Second)
	for {
		if err := eval(ctx, "window.__mf", &raw); err != nil {
			t.Fatalf("id read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the id scan never answered")
		}
		time.Sleep(400 * time.Millisecond)
	}
	var found struct {
		Found bool   `json:"found"`
		ID    string `json:"id"`
		Err   string `json:"err"`
	}
	if e := json.Unmarshal([]byte(raw), &found); e != nil {
		t.Fatalf("id payload: %v", e)
	}
	if !found.Found || found.ID == "" {
		t.Fatalf("the mention was sent but no message carries mentionedJidList (%s)", found.Err)
	}

	ms, err := message.New(runner, eval).MentionsOf(ctx, found.ID, "probe/mention")
	if err != nil {
		t.Fatalf("MentionsOf: %v", err)
	}
	t.Logf("read back: %s", ms)
	if len(ms.People) != 1 {
		t.Fatalf("want exactly the one mentioned person, got %d", len(ms.People))
	}
	if ms.People[0] != ident.JID {
		t.Fatal("the mention came back under a different identity than the one sent")
	}
	if len(ms.Groups) != 1 {
		t.Fatalf("want exactly one mentioned group, got %d", len(ms.Groups))
	}
	if ms.Groups[0].JID != gjid {
		t.Fatal("the mentioned group came back as a different group")
	}
	if ms.Groups[0].Subject == "" {
		t.Fatal("the group mention lost its subject, which is what the page renders")
	}
}

func containsOK(s string) bool {
	return len(s) > 0 && (s == `{"ok":true}` || (len(s) > 10 && s[:11] == `{"ok":true,`))
}
