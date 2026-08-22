package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/message"
	"wa-api/internal/wa-headless/capabilities/react"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeReactionRead applies H153's question to reactions: how does the
// REFERENCE obtain the data, and does that path exist here?
//
// H83 concluded the reaction aggregate "has no source on this build", which is
// why react.Remove cannot verify and MESSAGE_REACTION only says that reactions
// moved. The reference reads WAWebCollections.Reactions.find(msgId) keyed by
// msg.id._serialized (wwebjs_message.js:843-853).
//
// TWO THINGS MAKE THAT SUSPECT HERE, and they point opposite ways:
//   - `_serialized` is NULL on this build, so the reference's key does not
//     exist — the poll family already had to skip a conversion for this.
//   - `find` is broken on other collections here ("this.findImpl is not a
//     function"), measured on Chat and Newsletter.
//
// So this asks whether the failure is the KEY or the METHOD, because they have
// different remedies: a missing key can be substituted, a missing method cannot.
func TestProbeReactionRead(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_REACTREAD") == "" {
		t.Skip("set WA_PROBE_REACTREAD=1 (sends a message and reacts to it)")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
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

	sent, err := send.Text(ctx, runner, eval, peer, "wa-headless reaction probe", "probe/reactread")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	added, err := react.New(runner, eval).Add(ctx, sent.ID.ID, "\U0001F44D", "probe/reactread")
	if err != nil {
		t.Fatalf("react.Add: %v", err)
	}
	t.Logf("react.Add: %v", added)
	time.Sleep(6 * time.Second)

	m := message.New(runner, eval)
	after, err := m.ReactionsOf(ctx, sent.ID.ID, "probe/reactread/after")
	if err != nil {
		t.Fatalf("ReactionsOf after adding: %v", err)
	}
	t.Logf("AFTER ADD: %s", after)
	if !after.Any() {
		t.Fatal("the reader found no reaction on a message this session just reacted " +
			"to and verified; that is the exact state two earlier findings reported")
	}
	if !after.Groups[0].ByMe {
		t.Fatal("the page did not mark the reaction as ours, though we made it")
	}
	if len(after.Groups[0].Senders) == 0 {
		t.Fatal("the group carries no sender, so the reader cannot say WHO reacted")
	}

	// A TRANSICAO E' A PROVA. Uma leitura nao-vazia sozinha e' compativel com um
	// leitor que devolve constante; retirar e reler e' o controle.
	removed, err := react.New(runner, eval).Remove(ctx, sent.ID.ID, "probe/reactread/remove")
	if err != nil {
		t.Fatalf("react.Remove: %v", err)
	}
	t.Logf("react.Remove: %v", removed)
	// A METADE QUE FALTAVA. O `Verified:false` da remocao era falta de FONTE, e
	// a fonte existe agora; se ele voltar, e' regressao e nao contrato.
	if !removed.Verified {
		t.Fatal("react.Remove came back unverified, though the reactions record " +
			"is the postcondition it now waits on")
	}
	if removed.Has {
		t.Fatal("react.Remove reported the reaction still ours after taking it back")
	}
	time.Sleep(6 * time.Second)
	gone, err := m.ReactionsOf(ctx, sent.ID.ID, "probe/reactread/gone")
	if err != nil {
		t.Fatalf("ReactionsOf after removing: %v", err)
	}
	t.Logf("AFTER REMOVE: %s", gone)
	if gone.Total() >= after.Total() {
		t.Fatalf("the reading did not move when the reaction was taken back "+
			"(%d -> %d); a reader that never changes is not reading", after.Total(), gone.Total())
	}
	t.Log("PROVEN: reactions are readable, and the reading follows the fact")
}
