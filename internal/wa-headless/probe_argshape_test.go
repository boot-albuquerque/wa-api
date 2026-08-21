package waheadless

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
	"wa-api/internal/wa-headless/spa"
)

// TestProbeArgumentShapes runs the argument instrument against the functions
// this module is blocked on, and against two it already knows — the known ones
// are the CONTROL: an instrument that cannot reproduce a measured answer has not
// earned the unmeasured ones.
//
// It reads nothing about the account and sends nothing.
// argProbeDepth lets one run report top-level fields and the next report paths,
// without editing the test: depth is a question, not a setting.
func argProbeDepth() int {
	if v := os.Getenv("WA_PROBE_ARGSHAPE_DEPTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

// shapesJSON encodes the per-argument hints, or null when every argument should
// be probed as an object.
func shapesJSON(shapes []string) string {
	if len(shapes) == 0 {
		return "null"
	}
	b, err := json.Marshal(shapes)
	if err != nil {
		return "null"
	}
	return string(b)
}

func TestProbeArgumentShapes(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_ARGSHAPE") == "" {
		t.Skip("set WA_PROBE_ARGSHAPE=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	targets := []struct {
		mod, fn string
		arity   int
		shapes  []string
		note    string
	}{
		// CONTROLS: already known from readable sources.
		{"WAWebBlockContactAction", "blockContact", 1, nil,
			"known: {bizOptOutArgs, blockEntryPoint, contact, skipCtwa1pdNbfSignal}"},
		{"WAWebForwardMessagesToChat", "forwardMessagesToChats", 1, nil,
			"known: {msgs, chats, includeCaption, appendedText}"},
		// THE BLOCKED ONES.
		{"WAWebPollsSendPollCreationMsgAction", "sendPollCreation", 1, nil, "H69, closed"},
		{"WAWebEditLabelAssociationBridge", "editLabelAssociation", 2,
			[]string{"array", "array"}, "H72 — both arguments are lists"},
		{"WAWebTextStatusAction", "setMyTextStatus", 5,
			[]string{"string", "string", "string", "string", "string"}, "H66 — primitives"},
		// H57: the invite link. The blocker here is a HANG, not a shape — and a
		// hang is an answer too: the probe reporting "never settled" says the
		// call is waiting on something, while a thrown TypeError would say the
		// argument was wrong.
		{"WAWebReactionsUtils", "getReactionEmojisAndSum", 2,
			[]string{"array", "object"}, "ler reações"},
	}

	for _, tg := range targets {
		var raw string
		script := `JSON.stringify(await (` + spa.ArgumentProbeExpr + `)(` +
			strconv.Quote(tg.mod) + `, ` + strconv.Quote(tg.fn) + `, ` +
			strconv.Itoa(tg.arity) + `, ` + strconv.Itoa(argProbeDepth()) + `, ` +
			shapesJSON(tg.shapes) + `))`
		// Evaluate does not await promises, so the call is parked and polled —
		// the same shape every capability in this module uses.
		kick := `(() => { window.__argProbe = null;
			(async () => {
				try { window.__argProbe = ` + script + `; }
				catch (e) { window.__argProbe = JSON.stringify({ok:false, why:'OUTER: ' + String(e)}); }
			})();
			return 'started'; })()`
		var started string
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/arg-kick", func(c context.Context) error {
			return sess.Tab().Evaluate(c, kick, &started)
		}); err != nil {
			t.Errorf("%s.%s: kick: %v", tg.mod, tg.fn, err)
			continue
		}
		for i := 0; i < 40; i++ {
			if err := runner.Do(ctx, engine.OpStateProbe, "probe/arg-read", func(c context.Context) error {
				return sess.Tab().Evaluate(c, `window.__argProbe || ""`, &raw)
			}); err != nil {
				t.Errorf("%s.%s: read: %v", tg.mod, tg.fn, err)
				break
			}
			if raw != "" {
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if raw == "" {
			t.Errorf("%s.%s: never settled", tg.mod, tg.fn)
			continue
		}
		var out struct {
			OK    bool       `json:"ok"`
			Why   string     `json:"why"`
			Reads [][]string `json:"reads"`
			Threw string     `json:"threw"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Errorf("%s.%s: %v (raw %.200s)", tg.mod, tg.fn, err, raw)
			continue
		}
		if !out.OK {
			t.Logf("### %s.%s (%s)\n    REFUSED: %s", tg.mod, tg.fn, tg.note, out.Why)
			continue
		}
		var parts []string
		for i, r := range out.Reads {
			parts = append(parts, fmt.Sprintf("arg%d=[%s]", i, strings.Join(r, " ")))
		}
		t.Logf("### %s.%s (%s)\n    %s\n    threw: %s",
			tg.mod, tg.fn, tg.note, strings.Join(parts, "\n    "), out.Threw)
	}
}
