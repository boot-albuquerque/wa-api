package waheadless

import (
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/capabilities/send"
)

// TestProbeMarkReadIdentity answers the question decision 66 left open on my
// side, with the fixture the first attempt lacked.
//
// chats.MarkRead has the same shape as the three readers that refuse a phone jid
// — it looks the conversation up in the collection — and was NOT changed,
// because inferring from shape is exactly what produced this debt. The first
// measurement was inconclusive: both forms returned before=0, because the lab
// chat had nothing unread. A no-op is indistinguishable from a silent failure
// when there is nothing to do.
//
// SO THE UNREAD IS PRODUCED. conta-B sends, conta-A does not read, and then the
// phone jid is tried FIRST: if it silently does nothing, the unread survives for
// the lid call to find — and that difference is the whole answer.
func TestProbeMarkReadIdentity(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MARKREAD66") == "" {
		t.Skip("set WA_PROBE_MARKREAD66=1 (one message between the lab accounts)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	selfA, peerB := os.Getenv("WA_SELF_A_JID"), os.Getenv("WA_PEER_B_JID")
	if pa == "" || pb == "" || selfA == "" || peerB == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B, WA_SELF_A_JID and WA_PEER_B_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 12*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate

	if _, err := send.Text(ctx, d.RunnerB, evalB, selfA, "wa-headless markread-66 probe", "probe/mr66/send"); err != nil {
		t.Fatalf("conta-B send: %v", err)
	}
	time.Sleep(8 * time.Second)

	ident, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/mr66/resolve")
	if err != nil {
		t.Fatalf("resolving conta-B: %v", err)
	}
	c := chats.New(d.RunnerA, evalA)

	// O TELEFONE PRIMEIRO. Se ele fizer algo, a nao-lida some e a chamada
	// seguinte nao tem o que medir — a ordem e' o experimento.
	phone, errPhone := c.MarkRead(ctx, peerB, "probe/mr66/phone")
	t.Logf("MarkRead(phone) = %s err=%v", phone, errPhone)

	lid, errLid := c.MarkRead(ctx, ident.JID, "probe/mr66/lid")
	t.Logf("MarkRead(lid)   = %s err=%v", lid, errLid)

	if errLid != nil {
		t.Fatalf("MarkRead under the resolved identity failed (%v); without a "+
			"working side there is nothing to compare against", errLid)
	}
	switch {
	case phone.Before == 0 && lid.Before > 0:
		t.Fatalf("SILENT NO-OP: the phone jid reported nothing to acknowledge "+
			"(before=0) and the lid then found %d unread. MarkRead answers "+
			"SUCCESS for a call that did nothing, which is worse than the wrong "+
			"error the other three gave, and it must be refused too",
			lid.Before)
	case phone.Before > 0:
		t.Logf("MEDIDO: o jid de telefone FUNCIONA (before=%d) — MarkRead resolve "+
			"por um caminho que os outros tres nao usam, e nao deve ganhar a "+
			"recusa", phone.Before)
	default:
		t.Logf("INCONCLUSIVO: a conversa nao tinha nao-lidas quando a medicao "+
			"comecou (phone.before=%d lid.before=%d); sem nao-lida, no-op e falha "+
			"silenciosa sao indistinguiveis", phone.Before, lid.Before)
	}
}
