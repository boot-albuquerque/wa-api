package waheadless

import (
	"context"
	"os"
	"testing"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeLabelShape asks what labels look like on this build. Labels are a
// WhatsApp Business feature, and the lab account measured as a business account
// (H66), which is the only reason this is answerable here at all.
//
// Label NAMES are things the account's owner wrote, so they are reported as
// lengths. Ids and colours are structure and are shown.
func TestProbeLabelShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LABELS") == "" {
		t.Skip("set WA_PROBE_LABELS=1")
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
		try {
			const LC = window.require('WAWebLabelCollection').LabelCollection;
			const arr = LC.getModelsArray ? LC.getModelsArray() : [];
			out.count = arr.length;
			out.rows = arr.slice(0, 10).map(l => ({
				id: String(l.id),
				nameLen: (l.name || '').length,
				colorIndex: l.colorIndex,
				count: l.count,
				keys: Object.keys(l).filter(k => !k.startsWith('__')).slice(0, 14)
			}));
		} catch (e) { out.count = 'THREW:' + String(e).slice(0, 90); }

		// Which chats carry labels, as a COUNT.
		try {
			const CC = window.require('WAWebChatCollection').ChatCollection;
			let withLabels = 0, sample = null;
			for (const c of CC.getModelsArray()) {
				try {
					const ls = c.labels;
					if (ls && ls.length) {
						withLabels++;
						if (!sample) { sample = { labels: [].concat(ls).map(String), isGroup: c.id.server === 'g.us' }; }
					}
				} catch (e) {}
			}
			out.chatsWithLabels = withLabels;
			out.sampleChatLabels = sample;
		} catch (e) { out.chatsWithLabels = 'THREW'; }

		try {
			const B = window.require('WAWebEditLabelAssociationBridge');
			out.bridgeKeys = Object.keys(B);
			out.editSrc = typeof B.editLabelAssociation === 'function'
				? String(B.editLabelAssociation).slice(0, 400) : ('NOT_A_FUNCTION:' + typeof B.editLabelAssociation);
		} catch (e) { out.bridgeKeys = 'THREW'; }
		return out;
	})())`
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/labels", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("%s", raw)
}
