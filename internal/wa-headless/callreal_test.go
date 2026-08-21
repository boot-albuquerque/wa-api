package waheadless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/call"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
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
	t.Fatal("not wired: the trigger is WAWebVoipStartCall.startWAWebVoipCall (arity 5) and " +
		"the argument shape has not been measured. Measuring it means placing calls, which " +
		"is the thing this gate exists to require a decision about.")
}
