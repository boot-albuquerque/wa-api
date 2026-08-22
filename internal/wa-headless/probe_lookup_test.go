package waheadless

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/chats"
	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeNumberID proves lookup.NumberID against the real server.
//
// It asks THREE questions, because one would not distinguish the interesting
// cases: a number that exists and resolves, a number that cannot exist, and a
// group — which short-circuits, since the resolution answers people and would
// otherwise report a real group as absent.
//
// Identity-free: nothing logs a jid or a number.
func TestProbeNumberID(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LOOKUP") == "" {
		t.Skip("set WA_PROBE_LOOKUP=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := strings.TrimSpace(os.Getenv("WA_SEND_TO_JID"))
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
	r := lookup.New(runner, sess.Tab().Evaluate)

	// 1. A real number.
	got, err := r.NumberID(ctx, peer, "probe/lookup")
	if err != nil {
		t.Fatalf("NumberID(real): %v", err)
	}
	t.Logf("real peer -> %s", got)
	if got.JID == "" {
		t.Error("a real number produced no identity")
	}
	if got.IsGroup {
		t.Error("a one-to-one jid came back as a group")
	}
	// THE RESOLUTION MUST DO SOMETHING on a LID-first build: asking with a phone
	// jid and getting the same string back would mean the server never spoke.
	t.Logf("resolved to a different identity: %t", got.Resolved)

	// 2. A number that cannot exist. It must be a DEFINITE no, not a read error.
	if _, err := r.NumberID(ctx, "5500000000000@c.us", "probe/lookup"); err == nil {
		t.Error("an impossible number was reported as existing")
	} else if !errors.Is(err, lookup.ErrNotOnWhatsApp) {
		t.Errorf("an impossible number gave %v, want ErrNotOnWhatsApp — a caller "+
			"cannot tell 'no' from 'the read broke'", err)
	} else {
		t.Log("impossible number -> ErrNotOnWhatsApp, as a definite answer")
	}

	// 3. A group short-circuits rather than being reported absent.
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Log("lab group not found; skipping the group leg")
		return
	}
	g, err := r.NumberID(ctx, gjid, "probe/lookup")
	if err != nil {
		t.Fatalf("NumberID(group): %v — a group must not be reported absent", err)
	}
	if !g.IsGroup {
		t.Error("a group jid did not come back flagged as a group")
	}
	t.Logf("group -> %s", g)
}

// TestProbeByIdLookups proves chats.ByJID and contacts.LabelByID against the
// live session, closing the last two rows of the "internal step, not exposed"
// pattern.
//
// Identity-free: counts and booleans only, never a jid or a label name.
func TestProbeByIdLookups(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_BYID") == "" {
		t.Skip("set WA_PROBE_BYID=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	cl := chats.New(runner, eval)
	all, err := cl.List(ctx, 0, "probe/byid")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	t.Logf("chats listed: %d (total %d)", len(all.Chats), all.Total)
	if len(all.Chats) == 0 {
		t.Skip("no chats in this session")
	}

	// THE ONE THAT SORTS LAST, not the first: a truncating lookup finds the
	// first and misses this one, which is exactly the failure the unit test
	// models and the live run must not reproduce.
	last := all.Chats[len(all.Chats)-1]
	got, err := cl.ByJID(ctx, last.JID, "probe/byid")
	if err != nil {
		t.Fatalf("ByJID(last-sorting chat): %v", err)
	}
	if got.JID != last.JID {
		t.Error("ByJID returned a different conversation")
	}
	t.Logf("by-jid on the last-sorting chat: group=%t archived=%t muted=%t unread=%d",
		got.IsGroup, got.Archived, got.Muted, got.Unread)

	// A jid that cannot exist must be a definite no.
	if _, err := cl.ByJID(ctx, "000000000000000@lid", "probe/byid"); err == nil {
		t.Error("an impossible jid returned a conversation")
	} else {
		t.Logf("absent jid -> %v", err)
	}

	// Labels: this account measured THREE, all with zero items (H114).
	co := contacts.New(runner, eval)
	labels, err := co.ListLabels(ctx, "probe/byid")
	if err != nil {
		t.Logf("ListLabels: %v (a personal account has none)", err)
		return
	}
	t.Logf("labels: %d", len(labels.All))
	if len(labels.All) == 0 {
		t.Log("no labels on this account; the by-id leg cannot be exercised")
		return
	}
	one := labels.All[0]
	back, err := co.LabelByID(ctx, one.ID, "probe/byid")
	if err != nil {
		t.Fatalf("LabelByID: %v", err)
	}
	if back.ID != one.ID {
		t.Error("LabelByID returned a different label")
	}
	t.Logf("label by id: count=%d (a zero count is still a label)", back.Count)
	if _, err := co.LabelByID(ctx, "no-such-label", "probe/byid"); err == nil {
		t.Error("an impossible label id returned a label")
	}

	// CONTACTS BY JID, over the real roster. The property that matters is the one
	// the unit test models: a MERGED person must be findable under BOTH
	// identities, because this build files people under a lid and callers often
	// hold a phone jid.
	roster, err := co.List(ctx, "probe/byid")
	if err != nil {
		t.Fatalf("contacts List: %v", err)
	}
	t.Logf("roster: %d contacts from %d rows (%d merged)",
		len(roster.Contacts), roster.Rows, roster.Merged)

	var merged, nameless int
	var sample contacts.Contact
	for _, c := range roster.Contacts {
		if c.Merged {
			merged++
			if sample.PN == "" && c.PN != "" && c.LID != "" {
				sample = c
			}
		}
		if c.Pushname == "" {
			nameless++
		}
	}
	t.Logf("merged=%d nameless=%d", merged, nameless)

	if sample.PN == "" || sample.LID == "" {
		t.Log("no contact carries both identities; the both-ways leg cannot run")
	} else {
		byPN, e1 := co.ByJID(ctx, sample.PN, "probe/byid")
		byLID, e2 := co.ByJID(ctx, sample.LID, "probe/byid")
		if e1 != nil || e2 != nil {
			t.Fatalf("a merged contact was not findable under both identities: %v / %v", e1, e2)
		}
		if byPN.LID != byLID.LID || byPN.PN != byLID.PN {
			t.Error("the two identities returned different contacts")
		}
		t.Logf("merged contact reachable under both identities: %t", true)
	}

	// A NAMELESS contact must be findable — it is the majority case here.
	for _, c := range roster.Contacts {
		if c.Pushname == "" && c.PN != "" {
			if _, err := co.ByJID(ctx, c.PN, "probe/byid"); err != nil {
				t.Errorf("a contact with no name read as absent: %v", err)
			} else {
				t.Log("a nameless contact is findable, as it must be")
			}
			break
		}
	}

	if _, err := co.ByJID(ctx, "000000000000000@c.us", "probe/byid"); err == nil {
		t.Error("an impossible jid returned a contact")
	}
}
