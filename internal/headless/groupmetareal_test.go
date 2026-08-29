package headless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestGroupMetadataReal proves the four properties four ledger rows depend on:
// owner, createdAt, description and participants.
//
// READ ONLY. Nothing about the lab group changes.
func TestGroupMetadataReal(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_REAL_GROUPMETA") == "" {
		t.Skip("set WA_REAL_GROUPMETA=1; this only READS the lab group")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	runner := engine.NewRunner()
	h := waruntime.NewHolder(core.StartConfig{
		BinaryPath: findChrome(t), ProfileDir: profile, DebuggingPort: ephemeralPort(t),
		UserAgent: realSPAUserAgent, NavigateURL: realSPAURL, Runner: runner,
	})
	defer h.Stop(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}

	got, err := group.New(runner, sess.Tab().Evaluate).Metadata(ctx, gjid, "meta/read")
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	t.Logf("%s", got)
	for i, p := range got.Participants {
		t.Logf("  participant %d: %s", i+1, p)
	}

	// THE POSTCONDITIONS ARE WHAT A GROUP CANNOT LACK.
	if !strings.HasSuffix(got.JID, "@g.us") {
		t.Errorf("the jid is not a group jid")
	}
	if got.Subject == "" {
		t.Error("the group has no subject; the lab group is named, so this is the reader")
	}
	if got.CreatedAt.IsZero() {
		t.Error("the group has no creation time; every group has one")
	}
	if len(got.Participants) < 2 {
		t.Fatalf("%d participant(s); the lab group has at least two", len(got.Participants))
	}

	// EXACTLY ONE SUPER ADMIN, and it must be an admin too. A creator who is not
	// an admin would mean the two flags are being read from the same place.
	supers, admins := 0, 0
	for i, p := range got.Participants {
		if p.JID == "" {
			t.Errorf("participant %d has no jid", i)
		}
		if p.SuperAdmin {
			supers++
			if !p.Admin {
				t.Errorf("participant %d is super admin and not admin; the flags are conflated", i)
			}
		}
		if p.Admin {
			admins++
		}
	}
	if supers != 1 {
		t.Errorf("%d super admins; a group has exactly one creator", supers)
	}
	if got.Owner == "" {
		t.Error("the group has no owner; this account created the lab group")
	}

	// THE DESCRIPTION IS REPORTED, NOT ASSERTED. Both fields read undefined on
	// this group, which is consistent with "no description" and with "wrong
	// field", and one group cannot separate them — so the SOURCE is what gets
	// checked for coherence, not the text.
	t.Logf("description: source=%q len=%d descriptionAt=%t",
		got.DescriptionSource, len(got.Description), !got.DescriptionAt.IsZero())
	if got.DescriptionSource == "none" && got.Description != "" {
		t.Error("a description arrived with no source; the two must agree")
	}
	if got.DescriptionSource != "none" && got.Description == "" {
		t.Error("a source was named for an empty description; the two must agree")
	}
}
