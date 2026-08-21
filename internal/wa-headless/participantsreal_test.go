package waheadless

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPARemovesAndAddsTheLabPeer proves both directions ACROSS SESSIONS,
// because on this build there is no other honest way.
//
// A participant change reaches the server and is invisible to the session that
// made it: the group metadata keeps the old count (measured: 90 seconds, three
// runs) and no gp2/add or gp2/remove notice arrives, though a gp2/subject from a
// rename does — so the channel works and this particular notice simply is not
// delivered here.
//
// THE SINGLE-SESSION VERSION OF THIS TEST IS NOT MERELY WEAKER, IT IS UNSAFE.
// Removing and then adding in one session makes the second call read the stale
// metadata, conclude ALREADY_MEMBER, and do nothing — leaving the lab group with
// one member. That is exactly how it broke, and it is why each step below gets
// its own browser session.
func TestRealSPARemovesAndAddsTheLabPeer(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_GROUP_TEST") == "" {
		t.Skip("set WA_HEADLESS_GROUP_TEST=1; this removes and re-adds the peer lab account")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	// session runs one step against a FRESH browser session and returns.
	session := func(t *testing.T, what string, step func(context.Context, *group.Manager, string)) {
		t.Helper()
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
			t.Fatalf("%s: boot: %v", what, err)
		}
		m := group.New(runner, sess.Tab().Evaluate)
		gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
		if gjid == "" {
			t.Skipf("%s: lab group not found by subject", what)
		}
		step(ctx, m, gjid)
	}

	// The add is registered FIRST so it runs LAST, in its own session, whatever
	// the assertions in between do. A group left with one member breaks every
	// other live group test on this account.
	defer session(t, "restore", func(ctx context.Context, m *group.Manager, gjid string) {
		back, err := m.AddParticipant(ctx, gjid, peer, "parts/restore")
		if err != nil {
			t.Errorf("RESTORE FAILED — the lab group is left with one member and needs a human: %v", err)
			return
		}
		t.Logf("restored: %s", back)
	})

	var beforeCount int
	session(t, "remove", func(ctx context.Context, m *group.Manager, gjid string) {
		n, err := m.Count(ctx, gjid, "parts/count-before")
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		beforeCount = n
		if n != 2 {
			t.Fatalf("the lab group has %d participants, not the 2 this test needs", n)
		}
		got, err := m.RemoveParticipant(ctx, gjid, peer, "parts/remove")
		if err != nil {
			t.Fatalf("RemoveParticipant: %v", err)
		}
		t.Logf("removed: %s", got)
		if got.Verified {
			t.Fatal("a real participant change claims to be verified; this build cannot confirm one in-session")
		}
		if got.WantedAfter != 1 {
			t.Fatalf("the requested count is wrong: %s", got)
		}
	})

	session(t, "confirm", func(ctx context.Context, m *group.Manager, gjid string) {
		n, err := m.Count(ctx, gjid, "parts/count-after")
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		t.Logf("CROSS-SESSION: %d participants before the removal, %d after", beforeCount, n)
		if n != 1 {
			t.Fatalf("the removal did not take: %d participants, wanted 1", n)
		}
	})
}

// findLabGroupJID finds the lab group by its subject, synchronously.
func findLabGroupJID(ctx context.Context, t *testing.T, runner *engine.Runner,
	eval func(context.Context, string, *string) error) string {
	t.Helper()
	var jid string
	script := `(() => {
		const want = ` + strconv.Quote(labGroupSubject) + `;
		const CC = window.require('WAWebChatCollection').ChatCollection;
		for (const c of CC.getModelsArray()) {
			try {
				if (c && c.id && c.id.server === 'g.us') {
					const t = c.formattedTitle || c.name || c.subject;
					if (t === want && c.id._serialized) { return c.id._serialized; }
				}
			} catch (e) {}
		}
		return '';
	})()`
	if err := runner.Do(ctx, engine.OpStateProbe, "test/find-group", func(c context.Context) error {
		return eval(c, script, &jid)
	}); err != nil {
		t.Fatalf("finding the lab group: %v", err)
	}
	return jid
}
