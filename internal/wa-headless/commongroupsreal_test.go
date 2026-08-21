package waheadless

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAFindsCommonGroupsWithTheLabPeer reads only — nothing is sent and
// nothing changes.
//
// The account and the peer share exactly one group, the lab group, so the
// expected answer is known rather than merely plausible. And asking about THIS
// ACCOUNT must be refused, because the page returns null there and flattening
// null into "no groups" would answer a refusal with data.
func TestRealSPAFindsCommonGroupsWithTheLabPeer(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_READ_TEST") == "" {
		t.Skip("set WA_HEADLESS_READ_TEST=1; this only reads")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	l := contacts.New(runner, sess.Tab().Evaluate)
	got, err := l.CommonGroupsWith(ctx, peer, "test/common")
	if err != nil {
		t.Fatalf("CommonGroupsWith: %v", err)
	}
	t.Logf("with the peer: %s", got)
	if len(got.JIDs) == 0 {
		t.Fatal("no groups in common with the peer, and the lab group should be one")
	}
	for _, j := range got.JIDs {
		if len(j) < 5 || j[len(j)-5:] != "@g.us" {
			t.Fatalf("a returned identity is not a group jid (suffix %q)", j[max(0, len(j)-6):])
		}
	}

	// The lab group must be among them, found by subject in the same session.
	want := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if want != "" {
		found := false
		for _, j := range got.JIDs {
			if j == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the lab group is not among the %d common groups", len(got.JIDs))
		}
	}

	// ASKING ABOUT THIS ACCOUNT is a refusal, not an empty answer.
	self := ownJID(ctx, t, runner, sess.Tab().Evaluate)
	if self == "" {
		t.Skip("could not read this account's own jid; the self refusal was not exercised live")
	}
	if _, err := l.CommonGroupsWith(ctx, self, "test/common-self"); !errors.Is(err, contacts.ErrIsSelf) {
		t.Fatalf("asking about this account: got %v, want ErrIsSelf", err)
	}
	t.Log("asking about this account was refused, as the page's null requires")
}

// ownJID reads this account's own phone jid. It is never logged.
func ownJID(ctx context.Context, t *testing.T, runner *engine.Runner,
	eval func(context.Context, string, *string) error) string {
	t.Helper()
	var jid string
	script := `(() => {
		try {
			const Me = window.require('WAWebUserPrefsMeUser');
			// The same fallback list a probe measured earlier: this build does
			// not expose getMeUser, and picking one name would have skipped the
			// assertion silently — which it did, once.
			for (const k of ['getMaybeMeUser', 'getMeUser', 'getMe', 'getMeUserOrThrow']) {
				try {
					if (typeof Me[k] === 'function') {
						const u = Me[k]();
						if (u && u.user) { return u.user + '@c.us'; }
					}
				} catch (e) {}
			}
		} catch (e) {}
		return '';
	})()`
	if err := runner.Do(ctx, engine.OpStateProbe, "test/self", func(c context.Context) error {
		return eval(c, script, &jid)
	}); err != nil {
		t.Fatalf("reading this account's jid: %v", err)
	}
	return jid
}
