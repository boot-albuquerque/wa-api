package headless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/capabilities/lookup"
)

// TestProbeDirectJoin exercises the half of acceptInvite that H89 could not.
//
// The row proved the APPROVAL path live: joining a group with
// membership_approval_mode makes the page reject with
// UnexpectedJoinGroupViaInviteResponse, and that rejection IS the request being
// created. What was never exercised is the ordinary case — a group that lets
// people in — because the lab group requires approval, and turning that off on
// a fixture every other test depends on is not a change to make for one row.
//
// SO THE FIXTURE IS BUILT, NOT BORROWED. conta-A creates a throwaway group,
// confirms approval is off, and conta-B joins by code. Both accounts leave at the
// end, in a deferred step that runs whatever the assertions do — a group left
// behind would be a fixture nobody asked for, and the lab has enough of those.
func TestProbeDirectJoin(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_DIRECTJOIN") == "" {
		t.Skip("set WA_PROBE_DIRECTJOIN=1 (creates a throwaway group, joins it, leaves it)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	if pa == "" || pb == "" {
		t.Fatal("WA_PROFILE_A and WA_PROFILE_B are required")
	}
	d, ctx, done := openDual(t, pa, pb, 14*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate
	gA, gB := group.New(d.RunnerA, evalA), group.New(d.RunnerB, evalB)

	// O GRUPO NASCE COM CONTA-B E ELA SAI ANTES DE ENTRAR PELO CONVITE.
	//
	// Criar sozinha nao e' possivel — Ensure recusa com "a group needs at least
	// one participant", que e' a pagina falando e nao uma escolha nossa. Entao a
	// entrada por convite e' precedida por uma saida, e o que fica sob teste
	// continua sendo a ENTRADA por codigo e nao a adicao por participante.
	peerB := os.Getenv("WA_PEER_B_JID")
	if peerB == "" {
		t.Fatal("WA_PEER_B_JID is required")
	}
	identB, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/directjoin/resolve")
	if err != nil {
		t.Fatalf("resolving conta-B: %v", err)
	}
	// O ASSUNTO E' UNICO POR EXECUCAO, e isso foi aprendido caro: `Ensure` e'
	// idempotente POR ASSUNTO, entao um nome fixo faz a segunda rodada reusar o
	// grupo abandonado pela primeira — do qual esta conta ja' saiu. O sintoma foi
	// "this account is not an admin of that group" sobre um grupo recem-"criado",
	// com `created=false` na resposta dizendo exatamente isso a quem lesse.
	subject := "headless direct-join probe " + strconv.FormatInt(time.Now().Unix(), 10)
	created, err := gA.Ensure(ctx, subject, []string{identB.JID}, "probe/directjoin/create")
	if err != nil {
		t.Fatalf("creating the throwaway group: %v", err)
	}
	t.Logf("created: %s", created)
	if !created.Created {
		t.Fatalf("Ensure matched an existing group instead of creating one; the " +
			"subject was supposed to be unique to this run")
	}
	gjid := created.JID
	if gjid == "" {
		t.Fatal("the group came back without a jid")
	}
	defer func() {
		// AS DUAS SAEM, e conta-A por ultimo: sair primeiro como criadora pode
		// deixar conta-B sozinha num grupo que ninguem pediu.
		if err := gB.Leave(context.Background(), gjid, "probe/directjoin/leave-B"); err != nil {
			t.Logf("cleanup: conta-B leave: %v (expected if the join never landed)", err)
		}
		if err := gA.Leave(context.Background(), gjid, "probe/directjoin/leave-A"); err != nil {
			t.Errorf("CLEANUP FAILED, conta-A is still in the throwaway group: %v", err)
		} else {
			t.Log("CLEANED UP: both accounts left the throwaway group")
		}
	}()

	// A PROPAGACAO E' ESPERADA, nao presumida. A criacao volta com 2
	// participantes do lado de quem criou, e conta-B ainda nao tem o grupo:
	// pedir a saida nesse instante responde NO_CHAT, que e' verdade sobre a
	// sessao e nao sobre o grupo.
	seen := false
	waitB := time.Now().Add(60 * time.Second)
	for {
		if _, err := gB.Count(ctx, gjid, "probe/directjoin/wait-B"); err == nil {
			seen = true
			break
		}
		if time.Now().After(waitB) {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if !seen {
		t.Fatal("conta-B never received the group it was added to; the invite test " +
			"cannot start from here")
	}
	t.Log("conta-B received the group")

	// CONTA-B SAI, para que entrar signifique alguma coisa.
	if err := gB.Leave(ctx, gjid, "probe/directjoin/pre-leave"); err != nil {
		t.Fatalf("conta-B leaving before the invite test: %v", err)
	}
	time.Sleep(5 * time.Second)
	if n, err := gA.Count(ctx, gjid, "probe/directjoin/count-after-leave"); err == nil {
		t.Logf("after conta-B left, conta-A counts %d participant(s)", n)
	}

	// A PREMISSA E' CONFERIDA, nao assumida. Se um grupo novo ja' nascesse com
	// aprovacao ligada, este teste mediria de novo o caminho da H89 e chamaria
	// isso de caminho novo.
	needsApproval, err := gA.PolicyOf(ctx, gjid, group.PolicyJoinNeedsApproval, "probe/directjoin/policy")
	if err != nil {
		t.Fatalf("reading the approval policy: %v", err)
	}
	t.Logf("membership_approval_mode on the new group: %t", needsApproval)
	if needsApproval {
		if _, err := gA.SetPolicy(ctx, gjid, group.PolicyJoinNeedsApproval, false, "probe/directjoin/open"); err != nil {
			t.Fatalf("this group needs approval and it could not be turned off: %v", err)
		}
	}

	inv, err := gA.InviteCode(ctx, gjid, "probe/directjoin/code")
	if err != nil {
		t.Fatalf("getting the invite code: %v", err)
	}
	t.Logf("invite obtained: %s", inv)

	joined, err := gB.JoinByInvite(ctx, inv.Code, "probe/directjoin/join")
	if err != nil {
		t.Fatalf("conta-B joining by invite: %v", err)
	}
	t.Logf("conta-B JoinByInvite: %s", joined)
	if joined.Pending {
		t.Fatalf("the join came back PENDING on a group with approval off; that is " +
			"the H89 path again, not the direct one")
	}
	if joined.GroupJID != gjid {
		t.Fatal("the join reports a different group than the one invited to")
	}

	// A PROVA E' O GRUPO TER DOIS, lido do lado de quem ENTROU: "a chamada não
	// falhou" e "estou dentro" são fatos diferentes, e só o segundo fecha a linha.
	deadline := time.Now().Add(60 * time.Second)
	for {
		n, err := gB.Count(ctx, gjid, "probe/directjoin/count")
		if err == nil && n >= 2 {
			t.Logf("PROVEN: the direct join landed — conta-B reads %d participants", n)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the join returned without pending and conta-B does not see "+
				"itself in the group (count=%d err=%v)", n, err)
		}
		time.Sleep(3 * time.Second)
	}
}
