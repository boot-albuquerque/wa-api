package waheadless

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/call"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/events"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestCallLinkReal proves the half of the calls family that rings nobody.
//
// A call link is a URL. Creating one notifies no account and wakes no handset,
// which is why this runs freely while the rejection half waits for an explicit
// authorisation — see TestIncomingCallReal below.
//
// THE LINK IS NEVER LOGGED. It is a credential in the same sense a group invite
// code is: anybody holding it joins the call.
func TestCallLinkReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_CALLLINK") == "" {
		t.Skip("set WA_REAL_CALLLINK=1; this creates a real call link on conta-A")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	cm := call.New(runner, sess.Tab().Evaluate)

	for _, kind := range []call.Kind{call.KindVideo, call.KindVoice} {
		got, err := cm.CreateLink(ctx, time.Now().Add(2*time.Hour), kind, "call/link")
		if err != nil {
			t.Fatalf("CreateLink(%s): %v", kind, err)
		}
		t.Logf("%s: %s", kind, got)
		// THE POSTCONDITION IS THE SHAPE, checked without printing the link.
		// A page that answered "ok" with something that is not a URL would
		// otherwise pass, and the caller would publish it.
		if !strings.HasPrefix(got.URL, "https://") {
			t.Errorf("%s: the link is not a url", kind)
		}
		if len(got.URL) < 20 {
			t.Errorf("%s: the link is %d characters, which is not a joinable link", kind, len(got.URL))
		}
		if got.Kind != kind {
			t.Errorf("%s: the result says %s", kind, got.Kind)
		}
	}

	// The refusals, against the real page rather than a double: a kind the
	// upstream rejects must not reach the server here either.
	if _, err := cm.CreateLink(ctx, time.Now().Add(time.Hour), "audio", "call/bad-kind"); err == nil {
		t.Error("the real page accepted an unknown call kind")
	}

	// And the collection reader answers on an account with no calls.
	n, err := cm.Pending(ctx, "call/pending")
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	t.Logf("calls held by this session: %d", n)
}

// TestIncomingCallReal is the OTHER half, and it does not run without an
// explicit, separate authorisation.
//
// WHY IT IS GATED HARDER THAN EVERYTHING ELSE IN THIS SUITE. Proving Call.reject
// and the incoming-call event needs an incoming call, and this build CAN place
// one: the module scan found WAWebVoipStartCall.startWAWebVoipCall, which
// whatsapp-web.js has no equivalent for. But a placed call rings a PHYSICAL
// HANDSET — the one the lab account is paired with, in somebody's pocket. Every
// other outward effect in this suite lands in an app; this one makes hardware
// make noise.
//
// So it stays skipped by default, with the trigger named, rather than being
// quietly wired into a run somebody starts at night.
func TestIncomingCallReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_INCOMING_CALL") == "" {
		t.Skip("set WA_REAL_INCOMING_CALL=1 — THIS PLACES A REAL CALL between the lab " +
			"accounts and will RING A PHYSICAL PHONE. It exists so that Call.reject and " +
			"the incoming-call event can be proven; it is not part of any ordinary run.")
	}
	fromProfile := os.Getenv("WA_SEND_FROM_PROFILE")
	toProfile := os.Getenv("WA_SEND_TO_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if fromProfile == "" || toProfile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE, WA_SEND_TO_PROFILE and WA_SEND_TO_JID are required")
	}

	// conta-B FIRST, because it is the one that has to be LISTENING. A bus
	// installed after the call has already arrived proves nothing: the ingress
	// marks its first drain as replay precisely so that history cannot be
	// mistaken for news.
	runnerB := engine.NewRunner()
	hB := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: toProfile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runnerB,
	})
	defer hB.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sessB, err := hB.Session(ctx)
	if err != nil {
		t.Fatalf("conta-B boot: %v", err)
	}

	// CONTA-B HAS TO BE A CALL ENDPOINT, and a session that never initialised
	// its VOIP stack is not one. The first two attempts skipped this on the
	// receiving side and saw nothing; the application does it from a UI path a
	// headless driver does not have.
	if err := call.New(runnerB, sessB.Tab().Evaluate).EnsureReady(ctx, "call/b-ready"); err != nil {
		t.Fatalf("bringing conta-B's VOIP stack up: %v", err)
	}
	t.Log("conta-B is a call endpoint")

	hub := events.NewHub()
	defer hub.Close()
	var mu sync.Mutex
	var calls []events.Event
	seen := map[events.Type]int{}
	defer hub.Subscribe(func(e events.Event) {
		if e.Replay {
			return
		}
		mu.Lock()
		seen[e.Type]++
		if e.Type == events.CallIncoming {
			calls = append(calls, e)
		}
		mu.Unlock()
	})()
	pumpCtx, stopPump := context.WithCancel(ctx)
	defer stopPump()
	go func() { _ = events.NewPump(runnerB, sessB.Tab().Evaluate, hub).Run(pumpCtx) }()
	time.Sleep(3 * time.Second)

	// conta-A places the call.
	runnerA := engine.NewRunner()
	hA := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: fromProfile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runnerA,
	})
	defer hA.Stop(context.Background())
	sessA, err := hA.Session(ctx)
	if err != nil {
		t.Fatalf("conta-A boot: %v", err)
	}
	callerA := call.New(runnerA, sessA.Tab().Evaluate)

	// THE CANCEL IS REGISTERED BEFORE THE CALL IS PLACED. A test that dies
	// between dialling and rejecting would otherwise leave a phone ringing, and
	// "the run crashed" is not a reason for somebody's pocket to keep buzzing.
	// Registered after hA's Stop so it runs BEFORE it (defers are LIFO).
	defer func() {
		if err := callerA.Cancel(context.Background(), "call/cancel"); err != nil {
			t.Logf("cancelling the outgoing call: %v", err)
		}
	}()

	if err := callerA.Place(ctx, peer, false, "call/place"); err != nil {
		t.Fatalf("placing the call: %v", err)
	}
	t.Log("voice call placed from conta-A to conta-B")

	// DOES THE CALL EXIST AT ALL? Two different failures look identical from
	// conta-B: a call that was never placed, and a call that was placed and not
	// received. conta-A's own collection separates them without asking a human
	// whether a phone rang.
	placed := 0
	for i := 0; i < 10; i++ {
		if n, err := callerA.Pending(ctx, "call/a-pending"); err == nil && n > placed {
			placed = n
		}
		time.Sleep(time.Second)
	}
	t.Logf("conta-A holds %d call(s) after placing: a zero here means the dial did "+
		"nothing, and a non-zero means the call exists and conta-B is the side that "+
		"does not see it", placed)

	// The event, on conta-B.
	deadline := time.Now().Add(60 * time.Second)
	var got events.Event
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(calls) > 0 {
			got = calls[0]
		}
		n := len(calls)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	mu.Lock()
	snapshot := map[events.Type]int{}
	for k, v := range seen {
		snapshot[k] = v
	}
	mu.Unlock()
	t.Logf("bus on conta-B while the call rang: %v", snapshot)

	if got.Type != events.CallIncoming {
		// A NEGATIVE HERE IS A RESULT, NOT A CRASH. The reference observes calls
		// by patching an internal Map's set method, having apparently found no
		// listener; this build's collection exposes .on, so the clean door was
		// tried first. If it never fires, that is the measurement that would
		// justify considering the patch — and it belongs in the record either
		// way.
		reader, rerr := call.New(runnerB, sessB.Tab().Evaluate).Pending(ctx, "call/pending")
		t.Fatalf("no %s reached the bus in 60s.\n"+
			"  conta-B holds %d call(s) (err=%v); conta-A held %d after placing.\n"+
			"WHAT THIS DOES AND DOES NOT SHOW (H93): the dial resolves, the gating says "+
			"calling is enabled and the browser is fine, and ensureVoipInitialized "+
			"resolves on both sides. A human confirmed no handset rang on the first "+
			"attempt. But BOTH zeros above come from a reader that has never been "+
			"observed counting a call, so they cannot be used as a negative — testing "+
			"only the refusal path is the trap this repository catalogued. Separating "+
			"'the dial did nothing' from 'the reader counts nothing' needs a call that "+
			"is known to exist, which is the thing that is missing.",
			events.CallIncoming, reader, rerr, placed)
	}
	t.Logf("the call reached the bus: %s", got)
	if got.CallID == "" || got.CallerJID == "" {
		t.Fatalf("the event carries no call id or caller; a rejection needs both")
	}
	if got.OutgoingCall {
		t.Error("conta-B saw the call as outgoing; it received it")
	}
	if got.Video {
		t.Error("a voice call was reported as video")
	}

	// AND NOW THE REJECTION, which is the half that had no live proof at all.
	if err := call.New(runnerB, sessB.Tab().Evaluate).
		Reject(ctx, got.CallerJID, got.CallID, "call/reject"); err != nil {
		t.Fatalf("rejecting: %v", err)
	}
	t.Log("rejected from conta-B")

	// The rejection has no local postcondition — the doc on Reject says so — so
	// what is checked is the only thing that IS observable here: the call stops
	// being held by the receiving session.
	stopped := false
	until := time.Now().Add(45 * time.Second)
	for time.Now().Before(until) {
		n, err := call.New(runnerB, sessB.Tab().Evaluate).Pending(ctx, "call/after")
		if err != nil {
			t.Fatalf("reading pending calls: %v", err)
		}
		if n == 0 {
			stopped = true
			break
		}
		time.Sleep(3 * time.Second)
	}
	if !stopped {
		t.Error("conta-B still holds the call 45s after rejecting it")
	}
}
