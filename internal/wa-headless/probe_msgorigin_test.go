package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/message"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeMessageOrigin proves Message.getChat / Message.getContact against the
// messages this session actually loaded.
//
// It does not assert a fixed count, because how many messages hydrate is not
// ours to fix. It asserts the PROPERTY that makes the two ledger rows different
// from each other: on a group message the sender is NOT the chat. If every
// message read back with sender == chat, the reader would be a chat reader
// wearing two names.
//
// READ ONLY, identity-free: counts and booleans.
func TestProbeMessageOrigin(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MSGORIGIN") == "" {
		t.Skip("set WA_PROBE_MSGORIGIN=1")
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

	// Collect raw ids from the collection, keeping the group/one-to-one split so
	// both shapes get exercised.
	ids := loadedMessageIDs(ctx, t, runner, eval)
	if len(ids.group) == 0 && ids.direct == 0 {
		t.Skip("no messages loaded")
	}
	t.Logf("loaded: group=%d direct=%d", len(ids.group), ids.direct)

	r := message.New(runner, eval)
	var groupSenderDiffers, groupRead int
	for _, id := range ids.group {
		o, err := r.OriginOf(ctx, id, "probe/msgorigin")
		if err != nil {
			t.Logf("group message: %v", err)
			continue
		}
		groupRead++
		if !o.IsGroup {
			t.Errorf("a message from a group chat read back IsGroup=false")
		}
		if !o.SenderIsChat {
			groupSenderDiffers++
		}
	}
	t.Logf("group messages read=%d senderDiffersFromChat=%d", groupRead, groupSenderDiffers)
	if groupRead > 0 && groupSenderDiffers == 0 {
		t.Errorf("every group message reported the sender AS the chat: the reader is " +
			"not distinguishing getContact from getChat")
	}

	// THE SHAPE, on the same messages. Key names only; the values are never read,
	// which is what makes this a deliberate divergence from rawData rather than a
	// thin version of it.
	if len(ids.group) > 0 {
		keys, err := r.ShapeOf(ctx, ids.group[0], "probe/msgorigin")
		if err != nil {
			t.Errorf("ShapeOf: %v", err)
		} else {
			t.Logf("shape: %d field names; sample of the harmless ones: %v",
				len(keys), intersect(keys, []string{"id", "t", "type", "ack", "from", "to"}))
			for _, forbidden := range []string{"body", "caption", "text"} {
				if hasName(keys, forbidden) {
					t.Logf("the page DOES carry a %q field (name only; no value was read)", forbidden)
				}
			}
		}
	}

	// THE RE-READ, over a sample. A reload that reported the same thing about
	// every message would be a constant wearing a function's name, so the probe
	// asserts that it DISTINGUISHES: the measurement found ack spread over four
	// values and absent on five messages (H108).
	sample := ids.group
	sample = append(sample, ids.sampleDirect...)
	seen := map[string]int{}
	for _, id := range sample {
		c, err := r.CurrentOf(ctx, id, "probe/msgorigin")
		if err != nil {
			t.Logf("current: %v", err)
			continue
		}
		seen[c.String()]++
	}
	t.Logf("re-read %d messages, %d distinct states", len(sample), len(seen))
	for state, n := range seen {
		t.Logf("  %s x%d", state, n)
	}
	if len(sample) > 1 && len(seen) < 2 {
		t.Errorf("every message re-read identically: CurrentOf is not reporting " +
			"anything that varies")
	}

	// A raw id that was never loaded must be its own error, not a read failure.
	if _, err := r.OriginOf(ctx, "0000000000000000000000000000000000", "probe/msgorigin"); err == nil {
		t.Error("an id that cannot exist read back successfully")
	} else {
		t.Logf("absent id -> %v", err)
	}
}

type loadedIDs struct {
	group        []string
	direct       int
	sampleDirect []string
}

func loadedMessageIDs(ctx context.Context, t *testing.T, runner *engine.Runner,
	eval func(context.Context, string, *string) error) loadedIDs {
	t.Helper()
	const script = `(() => {
		window.__mo = null;
		try {
			const ms = window.require("WAWebMsgCollection").MsgCollection.getModelsArray();
			const group = [], direct = [];
			for (const m of ms) {
				const id = m.id;
				if (!id || !id.id) continue;
				const chat = String(id.remote || "");
				(chat.indexOf("@g.us") >= 0 ? group : direct).push(id.id);
			}
			window.__mo = JSON.stringify({group: group.slice(0, 12), directCount: direct.length,
				sampleDirect: direct.slice(0, 30)});
		} catch (e) {
			window.__mo = JSON.stringify({err: String((e && e.message) || e).slice(0, 120)});
		}
		return 'kicked';
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("collection kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		var raw string
		if err := eval(ctx, "window.__mo", &raw); err != nil {
			t.Fatalf("collection read: %v", err)
		}
		if raw != "" && raw != "null" {
			return parseLoadedIDs(t, raw)
		}
		if time.Now().After(deadline) {
			t.Fatal("the collection never answered")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func parseLoadedIDs(t *testing.T, raw string) loadedIDs {
	t.Helper()
	var v struct {
		Group        []string `json:"group"`
		DirectCount  int      `json:"directCount"`
		SampleDirect []string `json:"sampleDirect"`
		Err          string   `json:"err"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("collection payload: %v", err)
	}
	if v.Err != "" {
		t.Fatalf("collection: %s", v.Err)
	}
	return loadedIDs{group: v.Group, direct: v.DirectCount, sampleDirect: v.SampleDirect}
}

func hasName(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func intersect(hay, want []string) []string {
	var out []string
	for _, w := range want {
		if hasName(hay, w) {
			out = append(out, w)
		}
	}
	return out
}
