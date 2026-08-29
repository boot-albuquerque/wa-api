package headless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeEditShape asks the page what it will ALLOW and what it will SHOW,
// before anything is edited. It has no outward effect.
//
// The question that matters is the postcondition. sendMessageEdit's own body
// gives the refusal rule (canEditText / canEditCaption) but says nothing about
// how a successful edit becomes visible on the model — and a capability that
// cannot see its own effect reports the silent success this module refuses.
//
// It prints no message text: bodies are reported as LENGTHS.
func TestProbeEditShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_EDIT") == "" {
		t.Skip("set WA_PROBE_EDIT=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), nCycleReadyDeadline)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	var raw string
	script := `JSON.stringify((() => {
		const out = {};
		const Cap = window.require('WAWebMsgActionCapability');
		const U = window.require('WAWebMessageEditUtils');
		out.capKeys = Object.keys(Cap).filter(k => /[Ee]dit/.test(k));
		out.utilKeys = Object.keys(U);
		try { out.windowSeconds = U.getMessageEditProcessingWindowDurationSeconds(); } catch (e) { out.windowSeconds = 'THREW'; }
		try { out.editableTypes = Object.keys(U).length && U.MsgEditType ? Object.keys(U.MsgEditType) : null; } catch (e) {}

		const coll = window.require('WAWebMsgCollection').MsgCollection;
		const all = coll.getModelsArray();
		const rows = [];
		for (const m of all) {
			try {
				if (!m || !m.id || !m.id.fromMe || m.type !== 'chat') continue;
				rows.push({
					t: m.t || 0,
					bodyLen: (m.body || '').length,
					canEditText: !!(Cap.canEditText && Cap.canEditText(m)),
					canEditCaption: !!(Cap.canEditCaption && Cap.canEditCaption(m)),
					supportsType: !!(U.msgTypeSupportsEditing && U.msgTypeSupportsEditing(m.type)),
					inWindow: (() => { try { return !!U.isParentWithinEditProcessingWindow(m); } catch (e) { return 'THREW'; } })(),
					// The candidate postconditions: which of these exist on a
					// message that has NOT been edited tells which one changing
					// would mean something.
					hasLatestEditKey: m.latestEditMsgKey !== undefined,
					hasLatestEditTs: m.latestEditSenderTimestampMs !== undefined,
					hasIsEdited: m.isEdited !== undefined
				});
			} catch (e) {}
		}
		rows.sort((a, b) => b.t - a.t);
		out.fromMeTextCount = rows.length;
		out.newest = rows.slice(0, 5);
		return out;
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/edit", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("%s", raw)
}
