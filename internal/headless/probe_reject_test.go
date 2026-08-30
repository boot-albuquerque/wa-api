package headless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/capabilities/groupreq"
	"wa-api/internal/headless/capabilities/lookup"
)

// TestProbeRejectMembershipRequest exercises the row that has been PARTIAL for a
// reason that stopped being true when H164 showed how to build a fixture.
//
// rejectGroupMembershipRequests is implemented and locked by a unit test, and was
// never run live for a concrete reason: rejecting conta-B would expel it from the
// lab group, and that group is the fixture every other test leans on. Approve was
// exercised; reject was not, and "same RPC with a different key" is an argument,
// not a measurement.
//
// H164's answer applies unchanged: DO NOT BORROW THE FIXTURE, BUILD ONE. A
// throwaway group with approval turned ON gives conta-B something to request and
// conta-A something to reject, and nothing anybody depends on is touched.
func TestProbeRejectMembershipRequest(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_REJECT") == "" {
		t.Skip("set WA_PROBE_REJECT=1 (creates a throwaway group and rejects a join request)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	peerB := os.Getenv("WA_PEER_B_JID")
	if pa == "" || pb == "" || peerB == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B and WA_PEER_B_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 16*time.Minute)
	defer done()
	evalA, evalB := d.A.Tab().Evaluate, d.B.Tab().Evaluate
	gA, gB := group.New(d.RunnerA, evalA), group.New(d.RunnerB, evalB)

	identB, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/reject/resolve")
	if err != nil {
		t.Fatalf("resolving conta-B: %v", err)
	}
	subject := "headless reject probe " + strconv.FormatInt(time.Now().Unix(), 10)
	created, err := gA.Ensure(ctx, subject, []string{identB.JID}, "probe/reject/create")
	if err != nil {
		t.Fatalf("creating the throwaway group: %v", err)
	}
	gjid := created.JID
	// O defer VEM ANTES DE QUALQUER Fatal QUE O SIGA. Foi por registrá-lo tarde
	// que a H164 deixou dois grupos órfãos no servidor.
	defer func() {
		if err := gA.Leave(context.Background(), gjid, "probe/reject/cleanup"); err != nil {
			t.Errorf("CLEANUP FAILED, conta-A is still in the throwaway group: %v", err)
			return
		}
		t.Log("CLEANED UP: conta-A left the throwaway group")
	}()
	if !created.Created {
		t.Fatal("Ensure matched an existing group instead of creating one")
	}
	t.Logf("created: %s", created)

	// A CONDICAO ESPERADA E' SER MEMBRO, NAO CONSEGUIR LER.
	//
	// A primeira versao esperava `Count` responder e seguia; `Count` respondeu e
	// o `Leave` seguinte disse "this account is not a member of that group". Ler
	// o grupo e pertencer a ele sao fatos diferentes, e esperar pelo primeiro
	// para agir sobre o segundo e' a mesma confusao entre sessao e mundo que a
	// H162 custou caro para nomear.
	//
	// Entao a espera e' pela PROPRIA operacao: tenta sair ate' conseguir.
	waitB := time.Now().Add(90 * time.Second)
	for {
		err := gB.Leave(ctx, gjid, "probe/reject/pre-leave")
		if err == nil {
			break
		}
		if time.Now().After(waitB) {
			t.Fatalf("conta-B never became a member it could leave: %v", err)
		}
		time.Sleep(4 * time.Second)
	}
	t.Log("conta-B joined and left; it now has something to request")
	time.Sleep(5 * time.Second)

	if _, err := gA.SetPolicy(ctx, gjid, group.PolicyJoinNeedsApproval, true, "probe/reject/approval-on"); err != nil {
		t.Fatalf("turning approval on: %v", err)
	}
	inv, err := gA.InviteCode(ctx, gjid, "probe/reject/code")
	if err != nil {
		t.Fatalf("invite code: %v", err)
	}

	joined, err := gB.JoinByInvite(ctx, inv.Code, "probe/reject/request")
	t.Logf("conta-B JoinByInvite: %s err=%v", joined, err)
	// PEDIR E' O PONTO. Com aprovação ligada, a página recusa a entrada e a
	// recusa É o pedido sendo criado — o achado da H89, aqui reusado como
	// PRÉ-CONDIÇÃO em vez de conclusão.

	reqs := groupreq.New(d.RunnerA, evalA)
	var pending groupreq.List
	deadline := time.Now().Add(60 * time.Second)
	for {
		pending, err = reqs.List(ctx, gjid, "probe/reject/list")
		if err == nil && len(pending.Requests) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no membership request appeared for conta-A to reject "+
				"(err=%v); without one, rejecting proves nothing", err)
		}
		time.Sleep(3 * time.Second)
	}
	t.Logf("pending requests seen by conta-A: %d", len(pending.Requests))

	who := pending.Requests[0].RequesterJID
	out, err := reqs.Reject(ctx, gjid, []string{who}, "probe/reject/reject")
	if err != nil {
		t.Fatalf("Reject: %v", err)
	}
	for _, r := range out {
		t.Logf("Reject result: ok=%t code=%d why=%q", r.OK, r.Code, r.Why)
		if !r.OK {
			t.Fatalf("the page refused the rejection (code=%d why=%q)", r.Code, r.Why)
		}
	}

	// A PROVA E' O PEDIDO SUMIR E O GRUPO NAO CRESCER. "A chamada não falhou" e
	// "o pedido foi recusado" são fatos diferentes.
	deadline = time.Now().Add(60 * time.Second)
	for {
		after, err := reqs.List(ctx, gjid, "probe/reject/list-after")
		n, cerr := gA.Count(ctx, gjid, "probe/reject/count-after")
		if err == nil && len(after.Requests) == 0 {
			if cerr == nil && n > 1 {
				t.Fatalf("the request is gone and the group grew to %d; that is an "+
					"APPROVAL, not a rejection", n)
			}
			t.Logf("PROVEN: the request was rejected — 0 pending, group still %d", n)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the rejection returned ok and the request is still pending "+
				"(%d, err=%v)", len(after.Requests), err)
		}
		time.Sleep(3 * time.Second)
	}
}
