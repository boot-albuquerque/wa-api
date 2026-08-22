package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeMessageMutable measures what a re-read of a message could REPORT
// that a first read could not: the fields that change after the message exists.
//
// Message.reload in the reference re-fetches and returns null when the message
// is gone. Whether that is worth anything here depends on facts this probe
// measures rather than assumes: which ack values are actually present, whether
// any loaded message reads as revoked, and whether the revoked vocabulary this
// repo already uses (isRevokedMsg / type==='revoked' / revokeSender) agrees with
// itself on real data.
//
// READ ONLY, identity-free: counts and field names.
func TestProbeMessageMutable(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MSGMUTABLE") == "" {
		t.Skip("set WA_PROBE_MSGMUTABLE=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	const script = `(() => {
		window.__mm = null;
		try {
			const ms = window.require("WAWebMsgCollection").MsgCollection.getModelsArray();
			const acks = {}, types = {}, starred = {yes:0,no:0};
			let revokedByFlag = 0, revokedByType = 0, revokedBySender = 0, total = 0;
			for (const m of ms) {
				total++;
				const a = (typeof m.ack === "number") ? String(m.ack) : "absent";
				acks[a] = (acks[a] || 0) + 1;
				const ty = (typeof m.type === "string") ? m.type : "absent";
				types[ty] = (types[ty] || 0) + 1;
				starred[m.star ? "yes" : "no"]++;
				if (m.isRevokedMsg) { revokedByFlag++; }
				if (m.type === "revoked") { revokedByType++; }
				if (m.revokeSender) { revokedBySender++; }
			}
			window.__mm = JSON.stringify({total, acks, types, starred,
				revokedByFlag, revokedByType, revokedBySender});
		} catch (e) {
			window.__mm = JSON.stringify({err: String((e && e.message) || e).slice(0, 120)});
		}
		return 'kicked';
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__mm", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the collection never answered")
		}
		time.Sleep(200 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("mutable surface:\n%s", out)
}
