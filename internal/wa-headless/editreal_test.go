package waheadless

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/edit"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPAEditsAMessageItJustSent proves the edit against the live build.
//
// IT HAS TO SEND FIRST, and that is a measured requirement rather than a
// convenience: this build's edit window is 1200 seconds, and TestProbeEditShape
// found canEditText false for every message already in the collection. An edit
// test that reused an old message would fail for a reason that has nothing to
// do with the code.
//
// The outward effect is one short message to the peer lab account, edited once.
// There is no undo for an edit and none is attempted — a reader sees a message
// marked edited, which is the ordinary product behaviour, not damage.
func TestRealSPAEditsAMessageItJustSent(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_EDIT_TEST") == "" {
		t.Skip("set WA_HEADLESS_EDIT_TEST=1; this sends a message to the peer lab account and edits it")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// Two texts of DIFFERENT lengths, so the postcondition cannot pass by
	// comparing a body against itself.
	before := fmt.Sprintf("wa-headless edit probe %d", time.Now().UnixNano())
	after := before + " (edited)"

	sent, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer, before, "test/edit-send")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent: %s", sent)

	e := edit.New(runner, sess.Tab().Evaluate)
	got, err := e.Text(ctx, sent.ID.ID, after, "test/edit")
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	t.Logf("edited: %s", got)

	if got.ToLen != utf16Len(after) || got.FromLen != utf16Len(before) {
		t.Fatalf("the lengths do not match what was sent and asked for: %s", got)
	}
	if !got.Recorded {
		t.Error("the body changed but the page did not stamp the edit; a reader's client " +
			"would show the new text without marking it edited")
	}

	// EDITING SOMEBODY ELSE'S MESSAGE has to be refused, and the refusal has to
	// name the reason. The peer's own messages are in the same collection.
	var peerID string
	script := `(() => {
		const coll = window.require('WAWebMsgCollection').MsgCollection;
		let newest = null;
		for (const m of coll.getModelsArray()) {
			if (m && m.id && !m.id.fromMe && m.type === 'chat') {
				if (!newest || (m.t || 0) > (newest.t || 0)) { newest = m; }
			}
		}
		return newest ? newest.id.id : '';
	})()`
	if err := runner.Do(ctx, engine.OpStateProbe, "test/edit-peer", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &peerID)
	}); err != nil {
		t.Fatalf("finding a peer message: %v", err)
	}
	if peerID == "" {
		t.Skip("no inbound message loaded; the not-ours refusal was not exercised live")
	}
	if _, err := e.Text(ctx, peerID, "should never land", "test/edit-not-ours"); !errors.Is(err, edit.ErrNotOurs) {
		t.Fatalf("editing somebody else's message: got %v, want ErrNotOurs", err)
	}
}
