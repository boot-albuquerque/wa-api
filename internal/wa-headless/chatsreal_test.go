package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAListsChats proves the listing against a live account, and the
// assertion that matters is the TITLE COVERAGE.
//
// A listing whose titles are empty looks fine in every unit test — a double
// returns whatever it is told — and is useless to a person. Measured on this
// account: getName answers for 1 chat of 384, formattedTitle for 384. So the
// live proof checks that nearly every conversation came back with something to
// show, which is the one thing only a real account can say.
//
// No title is ever logged; only how many exist.
func TestRealSPAListsChats(t *testing.T) {
	requireRealSPA(t)
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Skip("WA_SEND_FROM_PROFILE is required")
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

	l := chats.New(runner, sess.Tab().Evaluate)
	got, err := l.List(ctx, 500, "real/chats")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	t.Logf("%s", got)

	if got.Total == 0 {
		t.Fatal("the account reported zero conversations; this one has hundreds, " +
			"so an empty read is a broken read")
	}
	if len(got.Chats) == 0 {
		t.Fatal("conversations were counted but none returned")
	}

	titled, groups, unread := 0, 0, 0
	for _, c := range got.Chats {
		if c.JID == "" {
			t.Fatal("a conversation came back with no identity")
		}
		if c.Title != "" {
			titled++
		}
		if c.IsGroup {
			groups++
		}
		if c.Unread > 0 {
			unread++
		}
	}
	t.Logf("of %d returned: titled=%d groups=%d withUnread=%d", len(got.Chats), titled, groups, unread)

	// THE ASSERTION THAT ONLY A REAL ACCOUNT CAN MAKE. With getName this would
	// have been 1; with formattedTitle it is all of them. A threshold rather
	// than equality, because a brand-new conversation may legitimately have
	// nothing to show yet.
	if titled*10 < len(got.Chats)*9 {
		t.Fatalf("only %d of %d conversations have a title. getName answers for 1 "+
			"of 384 on this build; formattedTitle for 384 — this reads like the "+
			"wrong field is being used (H51)", titled, len(got.Chats))
	}

	// ORDER, checked on real timestamps rather than trusted from the page.
	for i := 1; i < len(got.Chats); i++ {
		if got.Chats[i].Timestamp.After(got.Chats[i-1].Timestamp) {
			t.Fatalf("conversation %d is newer than %d: the listing is not ordered", i, i-1)
		}
	}
	if got.Truncated() {
		t.Fatalf("Truncated()=true while asking for more than the account holds "+
			"(%d of %d)", len(got.Chats), got.Total)
	}
}
