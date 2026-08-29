package headless

import (
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/contacts"
	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/capabilities/presence"
)

// TestProbeObservedPresence measures the half of sendPresenceAvailable and
// sendPresenceUnavailable that H50 could not: whether ANNOUNCING moves what the
// other side READS.
//
// H50 proved the announcement leaves and left the rows PARTIAL with the honest
// note "observação não provada; exige as duas contas na agenda uma da outra".
// That was true when it was written and stopped being true when the dual-session
// harness landed (H135) — the second account is exactly what was missing.
//
// THE PROOF IS A TRANSITION, NOT A READING. Observing online=true once proves
// nothing: it is also what a default, a stale cache, or a field this build never
// clears would say. So conta-B announces available, conta-A reads, conta-B
// announces unavailable, conta-A reads again — and the row only moves if the two
// readings DIFFER. That comparison is the negative control, built into the
// measurement instead of bolted on after it.
func TestProbeObservedPresence(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PRESENCE2") == "" {
		t.Skip("set WA_PROBE_PRESENCE2=1 (announces presence from the lab peer)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	peerB := os.Getenv("WA_PEER_B_JID")
	if pa == "" || pb == "" || peerB == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B and WA_PEER_B_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 8*time.Minute)
	defer done()

	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate
	annA := presence.New(d.RunnerA, evalA)
	annB := presence.New(d.RunnerB, evalB)

	// A IDENTIDADE OBSERVADA E' A RESOLVIDA. Observar um jid de telefone num
	// build LID-first e' observar alguem que a pagina nao arquiva sob esse id —
	// a licao da H136, que custou uma rodada inteira.
	ident, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/presence2")
	if err != nil {
		t.Fatalf("resolving the observed peer: %v", err)
	}

	// A CAUSA E' MEDIDA ANTES DE SER CULPADA. A nota da linha diz que a
	// observacao exige as duas contas na agenda uma da outra; se a assinatura
	// nao chegar, e' isso que tem de ser confirmado — nao presumido a partir da
	// falha, que e' compativel com varias causas.
	c := contacts.New(d.RunnerA, evalA)
	got, err := c.ByJID(ctx, ident.JID, "probe/presence2/book")
	t.Logf("conta-B in conta-A's address book: found=%t err=%v", err == nil, err)
	if err == nil {
		t.Logf("record: %s", got)
	}
	// O CONTACT DESTE MODULO NAO CARREGA O SINALIZADOR DA AGENDA, entao ele e'
	// lido cru — a pergunta e' "esta na agenda", nao "existe no WhatsApp", e sao
	// respostas diferentes.
	var book string
	bookScript := `(() => { try {
		const CC = window.require("WAWebContactCollection").ContactCollection;
		const c = CC.get(` + strconv.Quote(ident.JID) + `);
		if (!c) { return JSON.stringify({inCollection:false}); }
		return JSON.stringify({inCollection:true,
			isMyContact: !!c.isMyContact, isAddressBookContact: !!c.isAddressBookContact,
			isWAContact: !!c.isWAContact});
	} catch (e) { return JSON.stringify({err:String(e).slice(0,120)}); } })()`
	if err := evalA(ctx, bookScript, &book); err != nil {
		t.Fatalf("address-book read: %v", err)
	}
	t.Logf("address-book flags for the peer, seen by conta-A: %s", book)

	read := func(what string) presence.Snapshot {
		t.Helper()
		var last presence.Snapshot
		deadline := time.Now().Add(45 * time.Second)
		for {
			s, err := annA.Observe(ctx, ident.JID, "probe/presence2/"+what)
			if err == nil {
				last = s
				if s.Subscribed {
					return s
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("conta-A never got a subscribed presence for the peer (%v, last %s)", err, last)
			}
			time.Sleep(2 * time.Second)
		}
	}

	if err := annB.SetOnline(ctx, true, "probe/presence2/up"); err != nil {
		t.Fatalf("conta-B announcing available: %v", err)
	}
	time.Sleep(5 * time.Second)
	up := read("up")
	t.Logf("after conta-B announced AVAILABLE, conta-A reads: %s", up)

	if err := annB.SetOnline(ctx, false, "probe/presence2/down"); err != nil {
		t.Fatalf("conta-B announcing unavailable: %v", err)
	}
	time.Sleep(8 * time.Second)
	down := read("down")
	t.Logf("after conta-B announced UNAVAILABLE, conta-A reads: %s", down)

	if up.Online == down.Online {
		t.Fatalf("conta-A read Online=%t both times; the announcement does not move "+
			"what the other side sees, so the observation half stays unproven", up.Online)
	}
	if !up.Online {
		t.Fatal("the AVAILABLE announcement read as offline, which is the wrong direction")
	}
	t.Log("PROVEN: announcing presence moves what the other session reads")
}
