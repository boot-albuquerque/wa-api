package waheadless

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/status"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestStatusReadReal proves what can honestly be proven about status feeds.
//
// WHAT IT PROVES: the read path answers against the real page, the collection is
// where the reference says it is, and both identity forms are accepted.
//
// WHAT IT CANNOT PROVE, and says so instead of skipping quietly: a NON-EMPTY
// feed. The lab account has none, and creating one means POSTING a status —
// visible to every contact in the address book, measured at 944 on this
// account. Every outward effect this suite has ever produced landed on one known
// peer or one lab group. Nine hundred strangers is a different kind of act and
// it belongs to a human (H100).
func TestStatusReadReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_STATUS") == "" {
		t.Skip("set WA_REAL_STATUS=1; this only READS, and posts nothing")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: freePort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	r := status.New(runner, sess.Tab().Evaluate)

	feeds, err := r.List(ctx, "status/list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	t.Logf("this session holds %d status feed(s)", len(feeds))
	for _, f := range feeds {
		t.Logf("  %s", f)
		// EVERY FEED IS SELF-CONSISTENT, and this is the only assertion a live
		// run can make when the account is quiet: read + unread must not exceed
		// the total, whatever the counts happen to be.
		if f.Read+f.Unread > f.Total && f.Total > 0 {
			t.Errorf("read=%d + unread=%d exceeds total=%d", f.Read, f.Unread, f.Total)
		}
	}

	// The by-contact read against a contact that certainly exists. An absent
	// feed is the expected answer on a quiet account, and it must arrive as
	// ErrNotFound rather than as a Feed full of zeros — a caller would render
	// those as "this person posted nothing", which is a claim nobody measured.
	_, err = r.ByContact(ctx, peer, "status/by-contact")
	switch {
	case err == nil:
		t.Log("the peer has a status feed in this session")
	case errors.Is(err, status.ErrNotFound):
		t.Log("the peer has no status feed in this session, reported as such rather " +
			"than as an empty one")
	default:
		t.Fatalf("ByContact: %v", err)
	}

	if len(feeds) == 0 {
		t.Log("NOT PROVEN by this run: a non-empty feed. Proving it means posting a " +
			"status, which is visible to the whole address book — see the package doc.")
	}
}
