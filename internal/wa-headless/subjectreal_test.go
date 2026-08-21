package waheadless

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPARenamesTheLabGroup proves the rename and puts the name back.
//
// THE RESTORE IS NOT COSMETIC. Every other live group test finds the lab group
// BY ITS SUBJECT, so a run that renamed it and stopped would make the next one
// create a second group instead of reusing this one. The defer runs first
// because it is registered first.
//
// Renaming a group is visible to its members; the lab group's only other member
// is the peer lab account.
func TestRealSPARenamesTheLabGroup(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_GROUP_TEST") == "" {
		t.Skip("set WA_HEADLESS_GROUP_TEST=1; this renames the lab group")
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

	m := group.New(runner, sess.Tab().Evaluate)
	g, err := m.Ensure(ctx, labGroupSubject, []string{peer}, "subject/ensure")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	defer func() {
		back, err := m.SetSubject(context.Background(), g.JID, labGroupSubject, "subject/restore")
		if err != nil {
			t.Errorf("RESTORE FAILED — the lab group keeps a temporary name and the next "+
				"group test will create a second group instead of reusing it: %v", err)
			return
		}
		t.Logf("restored: %s", back)
	}()

	temp := fmt.Sprintf("%s [renomeado %d]", labGroupSubject, time.Now().Unix())
	got, err := m.SetSubject(ctx, g.JID, temp, "subject/set")
	if err != nil {
		t.Fatalf("SetSubject: %v", err)
	}
	t.Logf("renamed: %s", got)

	if got.ToLen != utf16Len(temp) {
		t.Fatalf("the new subject's length is %d and %d was asked for: %s", got.ToLen, utf16Len(temp), got)
	}
	if got.Field == "" {
		t.Fatal("the rename reports no field, so nothing was actually verified")
	}
	t.Logf("MEASURED: on this build a group's subject lives in chat.%s", got.Field)

	again, err := m.SetSubject(ctx, g.JID, temp, "subject/again")
	if err != nil {
		t.Fatalf("renaming to the same subject returned an error: %v", err)
	}
	if !again.AlreadyInState {
		t.Fatalf("a redundant rename was not reported as a no-op: %s", again)
	}
}
