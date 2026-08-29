package headless

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/capabilities/groupreq"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/events"
	waruntime "wa-api/internal/headless/runtime"
)

// TestMembershipRequestRoundTripReal proves the whole family against the real
// SPA, with no human in the loop.
//
// THE HARD PART IS NOT THE CALLS — IT IS PRODUCING A PENDING REQUEST. The lab
// group has none, and reading an empty list proves the path and not the shape.
// A request exists only when somebody follows a link into a group that asks for
// approval, so the proof has to MAKE one:
//
//	conta-A   turn membership approval on, take the invite code
//	conta-B   leave the group, follow the link -> becomes a pending request
//	conta-A   list it, approve it, and see it gone
//
// Everything it touches is a lab account and the lab group. It restores the
// approval mode it found, and conta-B ends where it started: in the group.
func TestMembershipRequestRoundTripReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_GROUPREQ") == "" {
		t.Skip("set WA_REAL_GROUPREQ=1; this makes conta-B leave and rejoin the lab group")
	}
	fromProfile := os.Getenv("WA_SEND_FROM_PROFILE")
	toProfile := os.Getenv("WA_SEND_TO_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if fromProfile == "" || toProfile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE and WA_SEND_TO_JID are required")
	}

	// One booted session at a time. Two browsers against two profiles would
	// work, but sequential keeps the failure reports readable: whatever breaks
	// breaks inside one named leg.
	leg := func(t *testing.T, what, profile string, step func(context.Context, *engine.Runner, *core.Session)) {
		t.Helper()
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
			t.Fatalf("%s: boot: %v", what, err)
		}
		step(ctx, runner, sess)
	}

	var groupJID, code string
	var approvalWas bool

	leg(t, "A/arm", fromProfile, func(ctx context.Context, runner *engine.Runner, sess *core.Session) {
		gm := group.New(runner, sess.Tab().Evaluate)
		groupJID = findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
		if groupJID == "" {
			t.Skip("lab group not found by subject")
		}
		var err error
		approvalWas, err = gm.PolicyOf(ctx, groupJID, group.PolicyJoinNeedsApproval, "greq/read-policy")
		if err != nil {
			t.Fatalf("reading the approval policy: %v", err)
		}
		if _, err := gm.SetPolicy(ctx, groupJID, group.PolicyJoinNeedsApproval, true, "greq/arm"); err != nil {
			t.Fatalf("turning approval on: %v", err)
		}
		inv, err := gm.InviteCode(ctx, groupJID, "greq/code")
		if err != nil {
			t.Fatalf("invite code: %v", err)
		}
		code = inv.Code
		if code == "" {
			t.Fatal("no invite code")
		}
		// The list must be EMPTY here. Without this leg the final assertion
		// could pass on a request that was already pending before the test ran.
		before, err := groupreq.New(runner, sess.Tab().Evaluate).List(ctx, groupJID, "greq/before")
		if err != nil {
			t.Fatalf("baseline list: %v", err)
		}
		// A LEFTOVER FROM A PREVIOUS RUN IS CLEARED, NOT TOLERATED.
		//
		// The first version of this leg refused to continue with anything
		// pending, which is right about provenance and wrong about rerunnability:
		// a run that fails after conta-B has requested leaves exactly one
		// pending request, and every later run then refuses at the door.
		//
		// So the baseline APPROVES what it finds. That is safe here and only
		// here: this is the lab group, it has two accounts, and the only way
		// something is pending in it is a previous run of this test. It is
		// logged loudly rather than done quietly.
		if len(before.Requests) > 0 {
			t.Logf("clearing %d leftover request(s) from an earlier run", len(before.Requests))
			ids := make([]string, 0, len(before.Requests))
			for _, r := range before.Requests {
				ids = append(ids, r.RequesterJID)
			}
			if _, err := groupreq.New(runner, sess.Tab().Evaluate).
				Approve(ctx, groupJID, ids, "greq/clear"); err != nil {
				t.Fatalf("clearing leftovers: %v", err)
			}
			// And the clear has to have WORKED, or the baseline is a lie.
			deadline := time.Now().Add(45 * time.Second)
			for {
				again, err := groupreq.New(runner, sess.Tab().Evaluate).List(ctx, groupJID, "greq/recheck")
				if err != nil {
					t.Fatalf("re-reading after the clear: %v", err)
				}
				if len(again.Requests) == 0 {
					break
				}
				if !time.Now().Before(deadline) {
					t.Fatalf("%d request(s) still pending after the clear", len(again.Requests))
				}
				time.Sleep(5 * time.Second)
			}
			// conta-B is now back in the group, which is where the next leg
			// expects it to start from.
			time.Sleep(5 * time.Second)
		}
		t.Logf("armed: approval was %t, baseline %s", approvalWas, before)
	})

	// Whatever happens next, put the policy back.
	t.Cleanup(func() {
		if groupJID == "" {
			return
		}
		leg(t, "A/restore", fromProfile, func(ctx context.Context, runner *engine.Runner, sess *core.Session) {
			gm := group.New(runner, sess.Tab().Evaluate)
			if _, err := gm.SetPolicy(ctx, groupJID, group.PolicyJoinNeedsApproval, approvalWas, "greq/restore"); err != nil {
				t.Errorf("restoring the approval policy to %t: %v", approvalWas, err)
			}
		})
	})

	leg(t, "B/request", toProfile, func(ctx context.Context, runner *engine.Runner, sess *core.Session) {
		gm := group.New(runner, sess.Tab().Evaluate)
		// READ THE LINK BEFORE FOLLOWING IT. It is the only chance to see what
		// the page says about a group that asks for approval, and it is the
		// half of the invite family that had no proof at all.
		info, err := gm.InviteInfo(ctx, code, "greq/info")
		if err != nil {
			t.Errorf("invite info: %v", err)
		} else {
			t.Logf("the link says: %s", info)
		}
		// A RERUN FINDS CONTA-B ALREADY OUT. The first live attempt left it
		// outside the group, and a leave that fails because there is nothing to
		// leave must not fail the run — the state this leg needs is "conta-B is
		// not a participant", and that is already true.
		if err := gm.Leave(ctx, groupJID, "greq/leave"); err != nil {
			t.Logf("leave reported %v; continuing, since what this leg needs is "+
				"conta-B OUT and a failed leave may mean it already is", err)
		}
		// The server needs a moment between a leave and a join by link;
		// following instantly gets refused as "already a participant".
		time.Sleep(5 * time.Second)
		joined, err := gm.JoinByInvite(ctx, code, "greq/join")
		switch {
		case errors.Is(err, group.ErrJoinPending):
			t.Logf("the join became a request, as the group asks: %s", joined)
		case err != nil:
			t.Fatalf("conta-B joining by link: %v", err)
		default:
			t.Fatalf("conta-B joined OUTRIGHT (%s); the approval policy did not take, "+
				"and this run proves nothing about membership requests", joined)
		}
	})

	leg(t, "A/answer", fromProfile, func(ctx context.Context, runner *engine.Runner, sess *core.Session) {
		mr := groupreq.New(runner, sess.Tab().Evaluate)
		var pending groupreq.List
		// The request travels through the server; A's session has to be told.
		deadline := time.Now().Add(90 * time.Second)
		for {
			var err error
			pending, err = mr.List(ctx, groupJID, "greq/list")
			if err != nil {
				t.Fatalf("listing requests: %v", err)
			}
			if len(pending.Requests) > 0 || !time.Now().Before(deadline) {
				break
			}
			time.Sleep(5 * time.Second)
		}
		if len(pending.Requests) == 0 {
			t.Fatal("no pending request appeared within 90s after conta-B followed the link")
		}
		// The field names are the measurement this family could not get from a
		// group with nothing pending. They are NAMES, never values.
		t.Logf("pending: %s; record fields = %v", pending, pending.Fields)

		// PROVENANCE COMES FROM THE BASELINE, NOT FROM MATCHING THE JID.
		//
		// The obvious check — "is this request conta-B's?" — would compare the
		// requester against WA_SEND_TO_JID, and it would be WRONG on this
		// build: the same account is @c.us in one place and @lid in another,
		// and the two user parts are different numbers, not different suffixes
		// on the same one (397 of 399 messages here are @lid). A jid compare
		// would call conta-B a stranger and fail a passing run.
		//
		// The baseline leg already asserted ZERO pending requests before conta-B
		// followed the link, which is the stronger claim: anything pending now
		// was made by this test. The peer comparison is kept as a LOG line,
		// because knowing which form the requester arrives in is exactly the
		// measurement this family was missing.
		if len(pending.Requests) != 1 {
			t.Fatalf("%d pending requests; the baseline said zero, so this run cannot "+
				"tell which one it made", len(pending.Requests))
		}
		target := pending.Requests[0].RequesterJID
		t.Logf("the requester arrives as %s and matches the peer's user part: %t",
			jidDomain(target), sameUser(target, peer))

		res, err := mr.Approve(ctx, groupJID, []string{target}, "greq/approve")
		if err != nil {
			t.Fatalf("approving: %v", err)
		}
		if len(res) != 1 || !res[0].OK {
			t.Fatalf("approve result = %v", res)
		}
		t.Logf("approved: %s", res[0])

		// AND THE REQUEST IS GONE. An approve that reports success and leaves
		// the request pending is the failure this assertion exists for — the
		// page answering about its own send rather than about the outcome.
		after := time.Now().Add(60 * time.Second)
		for {
			got, err := mr.List(ctx, groupJID, "greq/after")
			if err != nil {
				t.Fatalf("listing after approve: %v", err)
			}
			if len(got.Requests) == 0 {
				t.Logf("the request is gone after the approve")
				return
			}
			if !time.Now().Before(after) {
				t.Fatalf("the request is still pending %s after a successful approve", "60s")
			}
			time.Sleep(5 * time.Second)
		}
	})
}

// jidDomain is the SUFFIX of a jid — "@lid", "@c.us" — which says which
// identity space it lives in. It is the half of a jid that is safe to log.
func jidDomain(jid string) string {
	if i := strings.Index(jid, "@"); i >= 0 {
		return jid[i:]
	}
	return "(no domain)"
}

// sameUser compares two user jids by their user part, because the same account
// is @c.us in one place and @lid in another on this build (397 of 399 messages
// were @lid), and a string compare would call conta-B a stranger.
func sameUser(a, b string) bool {
	ua := strings.SplitN(strings.TrimSpace(a), "@", 2)[0]
	ub := strings.SplitN(strings.TrimSpace(b), "@", 2)[0]
	return ua != "" && ua == ub
}

// TestLabSetApprovalMode puts the lab group's approval policy where you say.
//
// It exists because the round trip ARMS real state and a failed run leaves it
// armed — which then becomes the "original" the next run restores to, and the
// lab group silently keeps a setting no test chose. That happened, and the way
// out was not a cleverer cleanup: it was a deterministic way to say what the
// group should be, independent of what it currently is.
func TestLabSetApprovalMode(t *testing.T) {
	requireRealSPA(t)
	want := os.Getenv("WA_LAB_APPROVAL")
	if want != "on" && want != "off" {
		t.Skip("set WA_LAB_APPROVAL=on|off")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}
	gm := group.New(runner, sess.Tab().Evaluate)
	before, err := gm.PolicyOf(ctx, gjid, group.PolicyJoinNeedsApproval, "lab/read")
	if err != nil {
		t.Fatalf("reading the policy: %v", err)
	}
	if _, err := gm.SetPolicy(ctx, gjid, group.PolicyJoinNeedsApproval, want == "on", "lab/set"); err != nil {
		t.Fatalf("setting the policy to %s: %v", want, err)
	}
	after, err := gm.PolicyOf(ctx, gjid, group.PolicyJoinNeedsApproval, "lab/verify")
	if err != nil {
		t.Fatalf("re-reading the policy: %v", err)
	}
	if after != (want == "on") {
		t.Fatalf("the policy reads %t after being set to %s", after, want)
	}
	// The participant count goes with it. The live proofs make conta-B leave
	// and rejoin, and a run that dies in the middle leaves it OUTSIDE — which
	// is a lab state worth seeing rather than discovering in the next test's
	// confusing failure.
	if n, err := gm.Count(ctx, gjid, "lab/count"); err != nil {
		t.Logf("participant count unavailable: %v", err)
	} else {
		t.Logf("participants in the lab group: %d", n)
	}
	t.Logf("approval mode: %t -> %t", before, after)
}

// TestMembershipRequestOnTheBusReal asks whether a request ARRIVING is
// something this bus can deliver.
//
// The family's method half is proven by the round trip above; this is its event
// half, and the orchestration's rule is that the two close together — a build
// that can answer requests but cannot notice them arriving makes every consumer
// poll.
//
// Unlike the round trip, both accounts are up AT THE SAME TIME. They have to
// be: an event is only observable while somebody is listening, and the round
// trip's sequential legs boot conta-A after the request already exists.
func TestMembershipRequestOnTheBusReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_GROUPREQ_BUS") == "" {
		t.Skip("set WA_REAL_GROUPREQ_BUS=1; this makes conta-B leave and rejoin the lab group")
	}
	fromProfile := os.Getenv("WA_SEND_FROM_PROFILE")
	toProfile := os.Getenv("WA_SEND_TO_PROFILE")
	if fromProfile == "" || toProfile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_PROFILE are required")
	}

	runnerA := engine.NewRunner()
	hA := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: fromProfile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runnerA,
	})
	defer hA.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	sessA, err := hA.Session(ctx)
	if err != nil {
		t.Fatalf("conta-A boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runnerA, sessA.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}
	gmA := group.New(runnerA, sessA.Tab().Evaluate)
	mrA := groupreq.New(runnerA, sessA.Tab().Evaluate)

	approvalWas, err := gmA.PolicyOf(ctx, gjid, group.PolicyJoinNeedsApproval, "bus/read-policy")
	if err != nil {
		t.Fatalf("reading the approval policy: %v", err)
	}
	if _, err := gmA.SetPolicy(ctx, gjid, group.PolicyJoinNeedsApproval, true, "bus/arm"); err != nil {
		t.Fatalf("arming approval: %v", err)
	}
	// A DEFER, NOT t.Cleanup, AND THE DIFFERENCE IS NOT STYLE. Cleanups run
	// AFTER every defer, so a cleanup that needs the session runs after
	// `defer hA.Stop` has already torn it down. The first version of this test
	// used t.Cleanup and failed with "context canceled" while the measurement
	// itself had succeeded — leaving the lab group armed.
	//
	// Defers run last-in-first-out, so this one, registered after the Stop,
	// runs before it, while the session is still alive.
	defer func() {
		if _, err := gmA.SetPolicy(context.Background(), gjid, group.PolicyJoinNeedsApproval,
			approvalWas, "bus/restore"); err != nil {
			t.Errorf("restoring approval to %t: %v", approvalWas, err)
		}
	}()
	inv, err := gmA.InviteCode(ctx, gjid, "bus/code")
	if err != nil {
		t.Fatalf("invite code: %v", err)
	}
	if before, err := mrA.List(ctx, gjid, "bus/baseline"); err != nil {
		t.Fatalf("baseline: %v", err)
	} else if len(before.Requests) > 0 {
		ids := make([]string, 0, len(before.Requests))
		for _, r := range before.Requests {
			ids = append(ids, r.RequesterJID)
		}
		if _, err := mrA.Approve(ctx, gjid, ids, "bus/clear"); err != nil {
			t.Fatalf("clearing leftovers: %v", err)
		}
		t.Logf("cleared %d leftover request(s)", len(ids))
		time.Sleep(5 * time.Second)
	}

	// The bus, on conta-A, watching everything.
	hub := events.NewHub()
	defer hub.Close()
	var mu sync.Mutex
	seen := map[events.Type]int{}
	defer hub.Subscribe(func(e events.Event) {
		if e.Replay {
			return
		}
		mu.Lock()
		seen[e.Type]++
		mu.Unlock()
	})()
	pumpCtx, stopPump := context.WithCancel(ctx)
	defer stopPump()
	pump := events.NewPump(runnerA, sessA.Tab().Evaluate, hub)
	go func() { _ = pump.Run(pumpCtx) }()
	// Let the pump install and burn its replay window before anything happens.
	time.Sleep(3 * time.Second)

	// conta-B, in its own browser, at the same time.
	runnerB := engine.NewRunner()
	hB := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: toProfile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runnerB,
	})
	defer hB.Stop(context.Background())
	sessB, err := hB.Session(ctx)
	if err != nil {
		t.Fatalf("conta-B boot: %v", err)
	}
	snap := func() map[events.Type]int {
		mu.Lock()
		defer mu.Unlock()
		out := map[events.Type]int{}
		for k, v := range seen {
			out[k] = v
		}
		return out
	}
	delta := func(before, after map[events.Type]int) map[events.Type]int {
		out := map[events.Type]int{}
		for k, v := range after {
			if d := v - before[k]; d > 0 {
				out[k] = d
			}
		}
		return out
	}

	gmB := group.New(runnerB, sessB.Tab().Evaluate)
	// THE MEASUREMENT IS SPLIT IN TWO, and that is the whole point of this
	// version. The first run counted the bus across a LEAVE followed by a JOIN
	// and reported 14 chat.changed — a number that says nothing, because a
	// departure moves the chat model too. Anything attributed to the request
	// had to survive the leave being subtracted from it.
	atStart := snap()
	if err := gmB.Leave(ctx, gjid, "bus/leave"); err != nil {
		t.Logf("leave reported %v; continuing if conta-B is already out", err)
	}
	time.Sleep(15 * time.Second)
	afterLeave := snap()
	t.Logf("bus for the LEAVE alone: %v", delta(atStart, afterLeave))

	if _, err := gmB.JoinByInvite(ctx, inv.Code, "bus/join"); err != nil && !errors.Is(err, group.ErrJoinPending) {
		t.Fatalf("conta-B joining: %v", err)
	}

	// Wait for conta-A to SEE the request at all, by the read that is already
	// proven. Without this the bus verdict cannot be trusted: no event and no
	// request means the request never arrived, which measures nothing.
	arrived := false
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		got, err := mrA.List(ctx, gjid, "bus/poll")
		if err != nil {
			t.Fatalf("polling requests: %v", err)
		}
		if len(got.Requests) > 0 {
			arrived = true
			break
		}
		time.Sleep(5 * time.Second)
	}
	if !arrived {
		t.Fatal("the request never reached conta-A; nothing can be concluded about the bus")
	}
	// Give the pump several cycles past the arrival.
	time.Sleep(5 * time.Second)

	t.Logf("bus for the REQUEST alone: %v", delta(afterLeave, snap()))

	// Leave the group as it was found: conta-B back inside.
	pending, err := mrA.List(ctx, gjid, "bus/final")
	if err != nil {
		t.Fatalf("final list: %v", err)
	}
	ids := make([]string, 0, len(pending.Requests))
	for _, r := range pending.Requests {
		ids = append(ids, r.RequesterJID)
	}
	if len(ids) > 0 {
		if _, err := mrA.Approve(ctx, gjid, ids, "bus/approve"); err != nil {
			t.Errorf("approving to restore conta-B: %v", err)
		}
	}
}
