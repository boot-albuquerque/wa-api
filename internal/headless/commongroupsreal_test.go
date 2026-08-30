package headless

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/ack"
	"wa-api/internal/headless/capabilities/contacts"
	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
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
		t.Skip("set HEADLESS_READ_TEST=1; this only reads")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
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

// TestRealSPAReadsThePeersAbout reads only. The about text is never logged —
// only its length — because it is something a person wrote about themselves.
func TestRealSPAReadsThePeersAbout(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_READ_TEST") == "" {
		t.Skip("set HEADLESS_READ_TEST=1; this only reads")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
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
	got, err := l.AboutOf(ctx, peer, "test/about")
	if err != nil {
		if errors.Is(err, contacts.ErrAboutDisabled) {
			t.Skip("this build has text status receiving disabled; the read path was not exercised")
		}
		t.Fatalf("AboutOf: %v", err)
	}
	t.Logf("peer about: %s", got)
	// WHAT THIS RUN DID AND DID NOT PROVE, said out loud so the PASS is not
	// read as more than it is. The peer's about came back EMPTY and already in
	// the collection, so what is proven is the cached-read path and the gate.
	// The FETCH path — going to the server — is not exercised by this, and
	// exercising it would mean querying a third party's about, which the lab
	// rules do not allow.
	if !got.Fetched {
		t.Log("NOT PROVEN by this run: the server-fetch path (the value was already cached)")
	}
	if len(got.Text) == 0 {
		t.Log("NOT PROVEN by this run: carrying a non-empty about (the peer has none)")
	}

	// ASKING TWICE MUST NOT REFETCH. The second call finds it in the
	// collection, which is what Fetched distinguishes — and a capability that
	// refetched every time would be a network call per read.
	again, err := l.AboutOf(ctx, peer, "test/about-again")
	if err != nil {
		t.Fatalf("second AboutOf: %v", err)
	}
	t.Logf("second read: %s", again)
	if again.Fetched && got.Fetched {
		t.Error("the second read went to the server again; the collection is not being consulted")
	}
	if again.Text != got.Text {
		t.Error("two reads of the same about returned different text")
	}
}

// TestRealSPAReadsTheAckOfAMessageItJustSent closes a loop: a message this
// session sent has an ack, and it is at least "sent".
//
// It asserts a FLOOR rather than an exact value on purpose. Whether the peer's
// device has acknowledged by the time this runs is a race with somebody else's
// phone, and a test that demanded "delivered" would be red for a reason that has
// nothing to do with this code.
func TestRealSPAReadsTheAckOfAMessageItJustSent(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_READ_TEST") == "" {
		t.Skip("set HEADLESS_READ_TEST=1; this sends one short message")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	sent, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer,
		fmt.Sprintf("headless ack probe %d", time.Now().UnixNano()), "test/ack-send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	r := ack.New(runner, sess.Tab().Evaluate)
	got, err := r.Of(ctx, sent.ID.ID, "test/ack")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	t.Logf("ack: %s", got)
	t.Logf("MEASURED: the state name came from %s", got.EnumSource)

	if !got.FromMe {
		t.Fatalf("a message this session sent is not marked fromMe: %s", got)
	}
	if got.State < ack.Sent {
		t.Fatalf("a message that send verified as delivered reads as %s: %s", got.State, got)
	}
	if got.State == ack.Unknown {
		t.Fatalf("the page's ack could not be named at all: %s", got)
	}

	// An id that is not loaded is its own answer.
	if _, err := r.Of(ctx, "3EBFFFFFFFFFFFFFFFFFFF", "test/ack-missing"); !errors.Is(err, ack.ErrNoMessage) {
		t.Fatalf("an unknown id: got %v, want ErrNoMessage", err)
	}
}

// TestRealSPAReadsTheBusinessLabels reads only.
//
// The lab account is a business account (H66, measured), which is why it has
// labels at all — three defaults, none applied to any chat. The assertions are
// about SHAPE rather than about those particular labels, because the account's
// owner may add or rename them and a test that pinned the names would break for
// a reason that has nothing to do with this code.
func TestRealSPAReadsTheBusinessLabels(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_READ_TEST") == "" {
		t.Skip("set HEADLESS_READ_TEST=1; this only reads")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
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
	got, err := l.ListLabels(ctx, "test/labels")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	t.Logf("labels: %s", got)
	for _, lab := range got.All {
		t.Logf("  %s", lab)
		if lab.ID == "" {
			t.Error("a label has no id")
		}
	}
	if len(got.All) == 0 {
		t.Log("NOT PROVEN by this run: reading a non-empty label set (this account has none)")
	}

	chatJID := findLabChatJID(ctx, t, runner, sess.Tab().Evaluate, peer)
	if chatJID == "" {
		t.Skip("no loaded chat with the peer")
	}
	ids, err := l.LabelsOfChat(ctx, chatJID, "test/chat-labels")
	if err != nil {
		t.Fatalf("LabelsOfChat: %v", err)
	}
	// An UNLABELLED chat must come back as an empty slice, not an error and not
	// nil — "this chat is not labelled" is an answer.
	if ids == nil {
		t.Fatal("an unlabelled chat returned nil instead of an empty slice")
	}
	t.Logf("the lab chat carries %d label(s)", len(ids))

	if _, err := l.LabelsOfChat(ctx, "15550009999@c.us", "test/chat-labels-missing"); !errors.Is(err, contacts.ErrNoSuchChatForLabels) {
		t.Fatalf("an unknown chat: got %v, want ErrNoSuchChatForLabels", err)
	}
}

// TestRealSPAAppliesAndRemovesALabel applies one of the account's own labels to
// the lab chat and takes it off again.
//
// The label vocabulary — "add" and "remove" — is the one part of the call that
// was NOT read from the page; it is inferred from the mirror call being named
// addOrRemoveLabelsMD. That is exactly why the postcondition reads chat.labels:
// a wrong verb produces a clean failure here instead of a silent no-op.
func TestRealSPAAppliesAndRemovesALabel(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_LABEL_TEST") == "" {
		t.Skip("set HEADLESS_LABEL_TEST=1; this labels and unlabels the lab chat")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
	}

	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
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
	labels, err := l.ListLabels(ctx, "test/label-list")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels.All) == 0 {
		t.Skip("this account has no labels to apply")
	}
	labelID := labels.All[0].ID

	chatJID := findLabChatJID(ctx, t, runner, sess.Tab().Evaluate, peer)
	if chatJID == "" {
		t.Skip("no loaded chat with the peer")
	}

	// Registered before the apply, so a failed assertion still takes the label
	// off the lab chat.
	defer func() {
		back, err := l.RemoveLabel(context.Background(), chatJID, labelID, "test/label-remove")
		if err != nil {
			t.Errorf("REMOVE FAILED — the lab chat keeps a label it did not have: %v", err)
			return
		}
		t.Logf("removed: %s", back)
	}()

	got, err := l.AddLabel(ctx, chatJID, labelID, "test/label-add")
	if err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	t.Logf("applied: %s", got)
	if got.NoOp {
		t.Fatal("the chat already carried this label, so this run proved nothing")
	}
	if got.After != got.Before+1 {
		t.Fatalf("the label count did not grow by exactly one: %s", got)
	}

	ids, err := l.LabelsOfChat(ctx, chatJID, "test/label-read-back")
	if err != nil {
		t.Fatalf("LabelsOfChat: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == labelID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the label was applied but does not read back on the chat (%d label(s) there)", len(ids))
	}

	again, err := l.AddLabel(ctx, chatJID, labelID, "test/label-add-again")
	if err != nil {
		t.Fatalf("applying a label the chat already has returned an error: %v", err)
	}
	if !again.NoOp {
		t.Fatalf("a redundant apply was not reported as a no-op: %s", again)
	}

	if _, err := l.AddLabel(ctx, chatJID, "no-such-label-id", "test/label-unknown"); !errors.Is(err, contacts.ErrNoLabelGiven) {
		t.Fatalf("an unknown label id: got %v, want ErrNoLabelGiven", err)
	}
}
