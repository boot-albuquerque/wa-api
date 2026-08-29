package headless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// labGroupSubject names the group so that a human who finds it on the account
// understands immediately what it is and that it can be removed.
//
// It is also the IDEMPOTENCY KEY: Ensure looks a group up by subject, so every
// run reuses this one instead of leaving another behind. Changing this string
// creates a second group.
const labGroupSubject = "wa-headless-lab — teste automatizado, pode apagar"

// TestRealSPASendsToAGroup is what H48 could not prove: the DISPATCH to a
// group, not just the resolution.
//
// It was impossible before because the only group on the account is a real one
// with real members, and a test that messages them is a test nobody may run. So
// this creates a group containing ONLY the two lab accounts and sends there.
//
// The group is created idempotently and never deleted by code — removing a
// group is a human action in the app, and doing it from here would be a
// destructive operation nobody asked for.
func TestRealSPASendsToAGroup(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_GROUP_TEST") == "" {
		t.Skip("set WA_HEADLESS_GROUP_TEST=1; this CREATES a lab group and sends to it")
	}
	from := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if from == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: from, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	// Minutes: creating a group is a server round trip (H42).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	g, err := group.New(runner, eval).Ensure(ctx, labGroupSubject, []string{peer}, "group/ensure")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	t.Logf("lab group: %s", g)
	if g.JID == "" {
		t.Fatal("the group has no identity")
	}
	// The account that created it is a member too, so a group holding only the
	// peer would mean the creator was left out.
	if g.Participants < 2 {
		t.Fatalf("the group reports %d participant(s); it must hold the two lab "+
			"accounts", g.Participants)
	}

	// IDEMPOTENCY, checked in the same run rather than assumed: asking again
	// must find the group, not make a second one.
	again, err := group.New(runner, eval).Ensure(ctx, labGroupSubject, []string{peer}, "group/ensure-again")
	if err != nil {
		t.Fatalf("Ensure (second call): %v", err)
	}
	if again.Created {
		t.Fatal("the second Ensure created ANOTHER group; every run would leave one " +
			"behind on a real account")
	}
	if again.JID != g.JID {
		t.Fatal("the second Ensure returned a different group")
	}
	t.Logf("second Ensure reused it: %s", again)

	// THE POINT: dispatch to the group. Before H48's fix this failed at the
	// identity step with NOT_ON_WHATSAPP.
	sendCtx, cancelSend := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancelSend()
	res, err := send.Text(sendCtx, runner, eval, g.JID,
		"wa-headless: teste automatizado de envio para grupo", "group/send")
	if err != nil {
		t.Fatalf("Text to group: %v", err)
	}
	t.Logf("SENT to the group and verified: %s", res)
	if res.ID.ID == "" {
		t.Fatal("the send returned no message id")
	}
}
