package headless

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/events"
)

// TestProbeGroupEventsFromAnObserver answers the question H119 left open and
// H86 could not reach: does a participant change reach a session that did NOT
// make it?
//
// H86 measured ZERO events in the acting session. H119 proved the gp2 carrier
// DOES reach this bus (a subject change fired group.updated). Between those two
// facts sits a question only two simultaneous sessions can answer, and this is
// it: conta-A watches while conta-B leaves the lab group and comes back.
//
// WHAT IT COSTS AND WHY THAT IS ACCEPTED: conta-B really leaves a real group and
// really rejoins. The lab group exists for exactly this, and the rejoin is
// registered with defer BEFORE the leave — a run that dies between them must
// still put conta-B back.
func TestProbeGroupEventsFromAnObserver(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_DUALGROUP") == "" {
		t.Skip("set WA_PROBE_DUALGROUP=1 (conta-B LEAVES and REJOINS the lab group)")
	}
	profileA := os.Getenv("WA_SEND_FROM_PROFILE")
	profileB := os.Getenv("WA_SEND_TO_PROFILE")
	if profileA == "" || profileB == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_PROFILE are required")
	}

	d, ctx, closeAll := openDual(t, profileA, profileB, 8*time.Minute)
	defer closeAll()
	t.Log("both accounts are awake at the same time")

	evalA := d.A.Tab().Evaluate
	evalB := d.B.Tab().Evaluate

	gjid := findLabGroupJID(ctx, t, d.RunnerA, evalA)
	if gjid == "" {
		t.Skip("lab group not found from the observer's side")
	}

	// THE OBSERVER'S BUS, running before anything happens.
	hub := events.NewHub()
	var mu sync.Mutex
	counts := map[events.Type]int{}
	subtypes := map[string]int{}
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		counts[e.Type]++
		if e.Subtype != "" {
			subtypes[e.Subtype]++
		}
	})
	defer unsub()
	pump := events.NewPump(d.RunnerA, evalA, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()

	// Let hydration drain, so what follows is attributable to the action.
	time.Sleep(10 * time.Second)
	mu.Lock()
	before := map[events.Type]int{}
	for k, v := range counts {
		before[k] = v
	}
	mu.Unlock()
	t.Logf("before the action: %v", before)

	// THE ACTOR. conta-B leaves; the rejoin is registered first.
	gmB := group.New(d.RunnerB, evalB)
	// THE CODE COMES FROM THE ADMIN SIDE, and getting that backwards cost a run:
	// conta-B is an ordinary member and reading an invite code requires admin.
	// The observer is the group's admin here, so A fetches the credential and
	// hands it to B — which is also what a human would do.
	gmA := group.New(d.RunnerA, evalA)
	code, err := gmA.InviteCode(ctx, gjid, "dual/code")
	if err != nil {
		t.Fatalf("the observer must be able to read the invite code so conta-B can "+
			"come back: %v", err)
	}
	rejoined := false
	defer func() {
		if rejoined {
			return
		}
		if _, err := gmB.JoinByInvite(ctx, code.Code, "dual/rejoin-undo"); err != nil &&
			!errors.Is(err, group.ErrJoinPending) {
			t.Errorf("REJOIN FAILED: conta-B is left OUT of the lab group: %v", err)
		}
	}()

	if err := gmB.Leave(ctx, gjid, "dual/leave"); err != nil {
		t.Fatalf("conta-B leaving: %v", err)
	}
	t.Log("conta-B left the group")

	// WATCH FROM A, bounded and reported rather than asserted.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		mu.Lock()
		got := counts[events.GroupLeft]
		mu.Unlock()
		if got > 0 {
			break
		}
	}
	mu.Lock()
	afterLeave := map[events.Type]int{}
	for k, v := range counts {
		afterLeave[k] = v
	}
	subsSnapshot := map[string]int{}
	for k, v := range subtypes {
		subsSnapshot[k] = v
	}
	mu.Unlock()
	t.Logf("after conta-B left, the OBSERVER saw: %v", afterLeave)
	t.Logf("subtypes seen: %v", subsSnapshot)

	if afterLeave[events.GroupLeft] > before[events.GroupLeft] {
		t.Logf("MEASURED: group.left DID reach the observer. H86's zero is about the " +
			"ACTING session only — an observer does receive participant events.")
	} else {
		t.Logf("MEASURED: no group.left reached the observer either. H86 generalises: " +
			"this build does not emit participant events to any session, not just to " +
			"the one that acted.")
	}

	// Put conta-B back and watch for the join from A.
	if _, err := gmB.JoinByInvite(ctx, code.Code, "dual/rejoin"); err != nil &&
		!errors.Is(err, group.ErrJoinPending) {
		t.Fatalf("conta-B rejoining: %v", err)
	}
	rejoined = true
	t.Log("conta-B is back in the group")

	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		mu.Lock()
		got := counts[events.GroupJoined]
		mu.Unlock()
		if got > 0 {
			break
		}
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("after conta-B rejoined, the OBSERVER saw: %v", counts)
	t.Logf("subtypes seen: %v", subtypes)
	if counts[events.GroupJoined] > 0 {
		t.Log("MEASURED: group.joined DID reach the observer.")
	} else {
		t.Log("MEASURED: no group.joined reached the observer.")
	}
}

// TestProbeAdminChangeFromAnObserver closes the third participant event.
//
// It flips the sides: conta-A ACTS (promotes and demotes) and conta-B OBSERVES.
// The shape matters — H86's finding is about the ACTING session, so the bus has
// to run on the other one, and the previous leg proved that is where the events
// land.
//
// conta-B is promoted to admin and demoted again on the lab group. The demote is
// registered with defer BEFORE the promote, so a run that dies in between does
// not leave a second admin standing.
func TestProbeAdminChangeFromAnObserver(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_DUALADMIN") == "" {
		t.Skip("set WA_PROBE_DUALADMIN=1 (promotes and demotes conta-B in the lab group)")
	}
	profileA := os.Getenv("WA_SEND_FROM_PROFILE")
	profileB := os.Getenv("WA_SEND_TO_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profileA == "" || profileB == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE and WA_SEND_TO_JID are required")
	}

	d, ctx, closeAll := openDual(t, profileA, profileB, 8*time.Minute)
	defer closeAll()

	evalA := d.A.Tab().Evaluate
	evalB := d.B.Tab().Evaluate
	gjid := findLabGroupJID(ctx, t, d.RunnerA, evalA)
	if gjid == "" {
		t.Skip("lab group not found")
	}

	// THE BUS RUNS ON B, the side that does NOT act.
	hub := events.NewHub()
	var mu sync.Mutex
	counts := map[events.Type]int{}
	subtypes := map[string]int{}
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		counts[e.Type]++
		if e.Subtype != "" {
			subtypes[e.Subtype]++
		}
	})
	defer unsub()
	pump := events.NewPump(d.RunnerB, evalB, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()
	time.Sleep(10 * time.Second)

	gmA := group.New(d.RunnerA, evalA)
	demoted := false
	defer func() {
		if demoted {
			return
		}
		if _, err := gmA.Demote(ctx, gjid, peer, "dual/demote-undo"); err != nil {
			t.Errorf("DEMOTE FAILED: conta-B is left as an admin of the lab group: %v", err)
		}
	}()

	if _, err := gmA.Promote(ctx, gjid, peer, "dual/promote"); err != nil {
		t.Fatalf("promoting conta-B: %v", err)
	}
	t.Log("conta-A promoted conta-B")

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		mu.Lock()
		got := counts[events.GroupAdminChanged]
		mu.Unlock()
		if got > 0 {
			break
		}
	}
	mu.Lock()
	afterPromote := counts[events.GroupAdminChanged]
	subsNow := map[string]int{}
	for k, v := range subtypes {
		subsNow[k] = v
	}
	mu.Unlock()
	t.Logf("after the promote, the OBSERVER saw %d %s; subtypes=%v",
		afterPromote, events.GroupAdminChanged, subsNow)

	if _, err := gmA.Demote(ctx, gjid, peer, "dual/demote"); err != nil {
		t.Fatalf("demoting conta-B: %v", err)
	}
	demoted = true
	t.Log("conta-A demoted conta-B")

	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		mu.Lock()
		got := counts[events.GroupAdminChanged]
		mu.Unlock()
		if got > afterPromote {
			break
		}
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("final: %v", counts)
	t.Logf("subtypes: %v", subtypes)
	if counts[events.GroupAdminChanged] > 0 {
		t.Logf("MEASURED: %s DID reach the observer (%d)", events.GroupAdminChanged,
			counts[events.GroupAdminChanged])
	} else {
		t.Logf("MEASURED: no %s reached the observer, while the leave and join legs "+
			"did — the difference would be about this subtype, not about the bus",
			events.GroupAdminChanged)
	}
}
