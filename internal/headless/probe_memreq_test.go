package headless

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/events"
)

// TestProbeMembershipRequestSignal asks whether a membership request carries a
// distinguishable word, which is the only thing standing between this row and a
// dedicated event type.
//
// H89 measured what arrives when a request lands: 9 chat.changed and 1
// message.added, against 5 chat.changed for a plain departure. The row concludes
// that chat.changed is too coarse to be the event, which is right — and it never
// asked what the message.added SAYS.
//
// THAT QUESTION HAS A PRECEDENT WITH A GOOD ANSWER. H119 turned the gp2 family
// into GROUP_JOIN / GROUP_LEAVE / GROUP_ADMIN_CHANGED / GROUP_UPDATE by carrying
// the page's own subtype across the boundary and classifying in Go — invariant 6
// respected, no decision made in the page. If a membership request arrives as a
// gp2 with its own subtype, the same machinery covers it and the row closes.
//
// The fixture is built, not borrowed (H164/H165): a throwaway group with approval
// on, requested by conta-B, and left behind by nobody.
func TestProbeMembershipRequestSignal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MEMREQ") == "" {
		t.Skip("set WA_PROBE_MEMREQ=1 (creates a throwaway group and a join request)")
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

	identB, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/memreq/resolve")
	if err != nil {
		t.Fatalf("resolving conta-B: %v", err)
	}
	subject := "headless memreq probe " + strconv.FormatInt(time.Now().Unix(), 10)
	created, err := gA.Ensure(ctx, subject, []string{identB.JID}, "probe/memreq/create")
	if err != nil {
		t.Fatalf("creating the throwaway group: %v", err)
	}
	gjid := created.JID
	defer func() {
		if err := gA.Leave(context.Background(), gjid, "probe/memreq/cleanup"); err != nil {
			t.Errorf("CLEANUP FAILED: %v", err)
		}
	}()
	if !created.Created {
		t.Fatal("Ensure matched an existing group instead of creating one")
	}

	waitB := time.Now().Add(90 * time.Second)
	for {
		if err := gB.Leave(ctx, gjid, "probe/memreq/pre-leave"); err == nil {
			break
		}
		if time.Now().After(waitB) {
			t.Fatal("conta-B never became a member it could leave")
		}
		time.Sleep(4 * time.Second)
	}
	if _, err := gA.SetPolicy(ctx, gjid, group.PolicyJoinNeedsApproval, true, "probe/memreq/approval"); err != nil {
		t.Fatalf("turning approval on: %v", err)
	}
	inv, err := gA.InviteCode(ctx, gjid, "probe/memreq/code")
	if err != nil {
		t.Fatalf("invite code: %v", err)
	}

	// O BARRAMENTO SO' ENTRA AGORA, depois de todo o preparo. Ligá-lo antes
	// encheria a medição com a criação, a saída e a mudança de política — que é
	// exatamente o erro que a H150 cometeu ao juntar três atos numa contagem.
	hub := events.NewHub()
	var mu sync.Mutex
	type row struct {
		Type                events.Type
		Kind, Subtype, Chat string
	}
	var seen []row
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, row{e.Type, e.Kind, e.Subtype, e.ChatJID})
	})
	defer unsub()
	pump := events.NewPump(d.RunnerA, evalA, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()
	time.Sleep(8 * time.Second)
	mu.Lock()
	base := len(seen)
	mu.Unlock()
	t.Logf("baseline events before the request: %d", base)

	if _, err := gB.JoinByInvite(ctx, inv.Code, "probe/memreq/request"); err == nil {
		t.Log("the join did not become a request; approval may not have taken")
	}

	time.Sleep(20 * time.Second)
	mu.Lock()
	fresh := append([]row(nil), seen[base:]...)
	mu.Unlock()

	tally := map[string]int{}
	for _, r := range fresh {
		key := string(r.Type)
		if r.Kind != "" || r.Subtype != "" {
			key += " kind=" + r.Kind + " subtype=" + r.Subtype
		}
		if r.Chat == gjid {
			key += " [this group]"
		}
		tally[key]++
	}
	t.Logf("events after the request: %d", len(fresh))
	for k, n := range tally {
		t.Logf("   %s  x%d", k, n)
	}

	// A PROVA E' O TIPO DEDICADO CHEGAR, e chegar SOZINHO: se o pedido viesse
	// junto com um group.updated, quem assinasse os dois contaria duas vezes.
	var requests, updates int
	for _, r := range fresh {
		if r.Chat != gjid {
			continue
		}
		switch r.Type {
		case events.GroupMembershipRequest:
			requests++
			if r.Subtype != "membership_approval_request" {
				t.Errorf("the request arrived with subtype %q", r.Subtype)
			}
		case events.GroupUpdated:
			updates++
		}
	}
	if requests != 1 {
		t.Fatalf("want exactly one %s for this group, got %d", events.GroupMembershipRequest, requests)
	}
	if updates != 0 {
		t.Fatalf("the request also produced %d %s; a subscriber to both would count "+
			"it twice", updates, events.GroupUpdated)
	}
	t.Logf("PROVEN: a join request arrives as %s, alone", events.GroupMembershipRequest)
}
