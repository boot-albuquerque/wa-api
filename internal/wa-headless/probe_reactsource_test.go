package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/react"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeReactionsCollectionFills answers whether the Reactions collection is
// a usable source for getReactions.
//
// The collection EXISTS with on/getModelsArray and measured EMPTY. Empty is not
// an answer on its own — this repository's own rule (H114) is that a zero needs
// a positive control before it becomes a conclusion. So this REACTS to a message
// and looks again, and removes the reaction afterwards.
//
// H83 had concluded there was no source for WHICH reaction. This does not
// contradict it blindly: it re-runs the question against a collection H83 may
// not have known about, and reports whichever way it falls.
func TestProbeReactionsCollectionFills(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_REACTFILL") == "" {
		t.Skip("set WA_PROBE_REACTFILL=1 (this REACTS to a message)")
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

	count := func(when string) int {
		const script = `(() => {
			window.__rq = null;
			try {
				const C = window.require("WAWebCollections");
				const col = C.Reactions;
				const n = (col && typeof col.getModelsArray === "function")
					? col.getModelsArray().length : -1;
				window.__rq = JSON.stringify({n: n});
			} catch (e) { window.__rq = JSON.stringify({n: -2}); }
			return "kicked";
		})()`
		var ignored string
		if err := eval(ctx, script, &ignored); err != nil {
			t.Fatalf("count kick (%s): %v", when, err)
		}
		for i := 0; i < 40; i++ {
			var raw string
			if err := eval(ctx, "window.__rq", &raw); err != nil {
				t.Fatalf("count read (%s): %v", when, err)
			}
			if raw != "" && raw != "null" {
				var v struct {
					N int `json:"n"`
				}
				if err := json.Unmarshal([]byte(raw), &v); err != nil {
					t.Fatalf("count payload (%s): %v", when, err)
				}
				return v.N
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("count never answered (%s)", when)
		return -3
	}

	// Pick a message of this account's own, so the reaction lands somewhere
	// harmless and removable.
	const pick = `(() => {
		window.__pm = null;
		try {
			const ms = window.require("WAWebCollections").Msg.getModelsArray();
			for (const m of ms) {
				if (m.type === "chat" && m.id && m.id.id && m.id.fromMe) {
					window.__pmid = m.id.id;
					window.__pm = JSON.stringify({found: true});
					return "kicked";
				}
			}
			window.__pm = JSON.stringify({found: false});
		} catch (e) { window.__pm = JSON.stringify({found: false}); }
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, pick, &ignored); err != nil {
		t.Fatalf("pick: %v", err)
	}
	var picked string
	for i := 0; i < 40; i++ {
		if err := eval(ctx, "window.__pm", &picked); err != nil {
			t.Fatalf("pick read: %v", err)
		}
		if picked != "" && picked != "null" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if picked == "" || picked == `{"found":false}` {
		t.Skip("no own chat message to react to")
	}
	var msgID string
	if err := eval(ctx, "window.__pmid", &msgID); err != nil {
		t.Fatalf("message id: %v", err)
	}

	before := count("before")
	t.Logf("Reactions collection before: %d", before)

	rr := react.New(runner, eval)
	if _, err := rr.Add(ctx, msgID, "👍", "probe/reactfill"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// THE REACTION IS REMOVED WHATEVER HAPPENS, by a defer registered right
	// after it lands and before anything that can fail.
	defer func() {
		if _, err := rr.Remove(ctx, msgID, "probe/reactfill-undo"); err != nil {
			t.Errorf("REMOVE FAILED: a reaction is left on a real message: %v", err)
		} else {
			t.Log("reaction removed")
		}
	}()

	after := count("after")
	t.Logf("Reactions collection after: %d", after)
	if after > before {
		t.Logf("MEASURED: the Reactions collection FILLS (%d -> %d). getReactions "+
			"has a source, and H83's negative was about something else.", before, after)
	} else {
		t.Logf("MEASURED: the collection stayed at %d after a reaction that the "+
			"capability verified. H83 stands: there is no per-message reaction "+
			"source here, only the flag that says they moved.", after)
	}
}
