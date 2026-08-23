package waheadless

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/addressbook"
	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeIdentityRefusal proves decision 66 where it matters — against the real
// build, with the real peer — and answers the one question the decision left
// open on my side.
//
// The decision named three readers because those are the three I MEASURED giving
// well-formed wrong answers. chats.MarkRead has the same shape and was never
// measured, so it was not changed: inferring from shape is exactly what produced
// this debt.
func TestProbeIdentityRefusal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_ID66") == "" {
		t.Skip("set WA_PROBE_ID66=1")
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
	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/id66/resolve")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}
	c := chats.New(runner, eval)
	ab := addressbook.New(runner, eval)

	// A RECUSA, com o jid de telefone do par real.
	if _, err := c.ByJID(ctx, peer, "probe/id66/byjid-phone"); !errors.Is(err, chats.ErrUnresolvedIdentity) {
		t.Fatalf("ByJID(phone) = %v, want ErrUnresolvedIdentity", err)
	}
	if _, err := c.MarkUnread(ctx, peer, "probe/id66/markunread-phone"); !errors.Is(err, chats.ErrUnresolvedIdentity) {
		t.Fatalf("MarkUnread(phone) = %v, want ErrUnresolvedIdentity", err)
	}
	if _, err := ab.DeviceCount(ctx, peer, "probe/id66/devices-phone"); !errors.Is(err, addressbook.ErrUnresolvedIdentity) {
		t.Fatalf("DeviceCount(phone) = %v, want ErrUnresolvedIdentity", err)
	}
	t.Log("os tres recusam o jid de telefone, e a recusa tem erro proprio")

	// E A RESPOSTA, sob a identidade resolvida. Uma recusa que tambem recusasse o
	// lid seria "sempre falha", que passa no teste da recusa e nao serve a
	// ninguem — este e' o controle positivo dela.
	if _, err := c.ByJID(ctx, ident.JID, "probe/id66/byjid-lid"); err != nil {
		t.Fatalf("ByJID(lid) = %v, want the conversation", err)
	}
	n, err := ab.DeviceCount(ctx, ident.JID, "probe/id66/devices-lid")
	if err != nil {
		t.Fatalf("DeviceCount(lid) = %v, want a count", err)
	}
	t.Logf("sob o lid: ByJID acha a conversa, DeviceCount devolve %d", n)

	// A PERGUNTA EM ABERTO: o MarkRead tem a mesma forma e nao foi medido.
	// O ERRO NIL NAO PROVA QUE FEZ ALGO. Um MarkRead que nao acha a conversa pode
	// ler zero nao-lidas e retornar "nada a reconhecer" — sucesso silencioso, que
	// e' PIOR que o erro errado dos outros tres. O veredito sai do Result.
	resPhone, errPhone := c.MarkRead(ctx, peer, "probe/id66/markread-phone")
	t.Logf("MarkRead(phone) = %s err=%v", resPhone, errPhone)
	resLid, errLid := c.MarkRead(ctx, ident.JID, "probe/id66/markread-lid")
	t.Logf("MarkRead(lid)   = %s err=%v", resLid, errLid)
	switch {
	case errPhone != nil && errLid == nil:
		t.Log("MEDIDO: MarkRead responde diferente por identidade — mesma familia " +
			"dos tres, e a recusa deve ser estendida a ele")
	case errPhone == nil && errLid == nil && resPhone.String() == resLid.String():
		t.Log("MEDIDO: MarkRead responde IGUAL para as duas formas — nao pertence " +
			"ao grupo dos que respondem errado, e nao deve ganhar a recusa")
	default:
		t.Logf("MEDIDO: as duas chamadas passaram e os resultados DIFEREM "+
			"(phone=%s lid=%s). Se o do telefone for um no-op que se declara "+
			"sucesso, isso e' sucesso silencioso e pior que os outros tres.",
			resPhone, resLid)
	}
}
