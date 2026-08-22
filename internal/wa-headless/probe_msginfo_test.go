package waheadless

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/message"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeMessageInfo re-measures getInfo with the instrument the reference
// actually uses.
//
// H71 measured MsgInfoCollection EMPTY — 0 of 368 — and concluded we have ack,
// not "who read it". The measurement was honest and pointed at the wrong thing:
// the reference does not read that collection at all. It calls
// WAWebApiMessageInfoStore.queryMsgInfo(msg.id) (wwebjs_message.js:758-781), an
// explicit query. A collection populated BY a query reads empty until somebody
// queries — which is the same shape as H142's mentions, and the same lesson.
//
// It also carries a constraint H71 could not have honoured, because it lives in
// the reference and not in the collection: `if (!msg.id.fromMe) return null`.
// Message info is only about THIS account's own messages.
//
// A GROUP MESSAGE IS THE INTERESTING CASE. In a one-to-one, "who read it" adds
// nothing over ack. In a group it is per participant, which is the whole point
// of the method.
func TestProbeMessageInfo(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MSGINFO") == "" {
		t.Skip("set WA_PROBE_MSGINFO=1 (sends one message to the lab group)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
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

	// PRIMEIRO O MODULO EXISTE? Tres nomes inventados falharam esta semana; a
	// enumeracao vem antes da chamada (H143).
	var surface string
	const enumerate = `(() => {
		const out = {};
		for (const m of ["WAWebApiMessageInfoStore","WAWebMsgInfoCollection"]) {
			try { const mod = window.require(m); out[m] = mod ? Object.keys(mod).slice(0, 10) : "empty"; }
			catch (e) { out[m] = "absent"; }
		}
		try {
			const C = window.require("WAWebCollections");
			out.msgInfoInCollections = typeof C.MsgInfo !== "undefined";
			if (C.MsgInfo && typeof C.MsgInfo.getModelsArray === "function") {
				out.msgInfoSize = C.MsgInfo.getModelsArray().length;
			}
		} catch (e) { out.collErr = String(e).slice(0, 80); }
		return JSON.stringify(out);
	})()`
	if err := eval(ctx, enumerate, &surface); err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	t.Logf("message-info surface: %s", surface)

	gjid := findLabGroupJID(ctx, t, runner, eval)
	if gjid == "" {
		t.Skip("lab group not found; per-participant info needs a group")
	}
	sent, err := send.Text(ctx, runner, eval, gjid, "wa-headless msginfo probe", "probe/msginfo")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent: %s", sent)
	// O RELOGIO E' DAQUI. A referencia dorme DENTRO da pagina quando a mensagem
	// tem menos de 1250ms; a invariante 6 manda essa espera ficar no Go.
	time.Sleep(6 * time.Second)

	m := message.New(runner, eval)
	got, err := m.InfoOf(ctx, sent.ID.ID, "probe/msginfo")
	if err != nil {
		t.Fatalf("InfoOf: %v", err)
	}
	t.Logf("InfoOf on our own group message: %s", got)
	if !got.Answered {
		t.Fatal("the store did not answer about a message sent six seconds ago; " +
			"without an answer nothing below means anything")
	}
	if len(got.Delivered) == 0 && got.DeliveredRemaining <= 0 {
		t.Fatal("the info came back with nobody delivered and nothing remaining, " +
			"which is not a state a just-sent group message can be in")
	}

	// A RECUSA TAMBEM E' PROVA. Uma capacidade so' vista aceitando nao esta
	// provada — a licao da H147, aplicada no mesmo dia em que foi escrita.
	other := findIncomingMessageID(ctx, t, eval)
	if other == "" {
		t.Log("no incoming message loaded; the refusal path stays unexercised here")
		return
	}
	if _, err := m.InfoOf(ctx, other, "probe/msginfo/other"); !errors.Is(err, message.ErrNotMine) {
		t.Fatalf("asking about somebody else's message answered %v instead of ErrNotMine", err)
	}
	t.Log("PROVEN: the store answers about our own message and refuses somebody else's")
}

// findIncomingMessageID returns the raw id of a loaded message this account did
// NOT send, or "" when the session has none.
func findIncomingMessageID(ctx context.Context, t *testing.T, eval func(context.Context, string, *string) error) string {
	t.Helper()
	var raw string
	const script = `(() => {
		try {
			const ms = window.require("WAWebCollections").Msg.getModelsArray();
			for (const m of ms) {
				if (m.id && !m.id.fromMe && m.id.id) { return m.id.id; }
			}
			return "";
		} catch (e) { return ""; }
	})()`
	if err := eval(ctx, script, &raw); err != nil {
		t.Fatalf("finding an incoming message: %v", err)
	}
	return raw
}
