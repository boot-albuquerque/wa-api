package waheadless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeMentions measures where a message keeps its mentions.
//
// The reference reads this.mentionedIds and this.groupMentions off the message
// model. Whether those names survive on this build is the question, and this
// build has hidden a value behind a different name five times (H83, H94, H103,
// H104, H105).
//
// READ ONLY, and identity-free: it counts mentions and reports FIELD NAMES and
// jid DOMAINS. A mention is a person.
func TestProbeMentions(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MENTIONS") == "" {
		t.Skip("set WA_PROBE_MENTIONS=1")
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

	const script = `JSON.stringify((() => {
		const out = { scanned: 0, withMentions: 0, withGroupMentions: 0, candidates: {} };
		try {
			const MC = window.require('WAWebMsgCollection').MsgCollection;
			const all = typeof MC.getModelsArray === 'function' ? MC.getModelsArray() : [];
			out.scanned = all.length;
			// TODOS OS NOMES PLAUSIVEIS, porque o nome da referencia ja falhou
			// cinco vezes neste build. Conta quantas mensagens tem cada um.
			const names = ['mentionedIds', 'mentionedJids', 'groupMentions',
				'mentions', 'quotedParticipant'];
			for (const n of names) { out.candidates[n] = 0; }
			let sample = null, groupSample = null;
			for (const m of all) {
				for (const n of names) {
					try {
						const v = m[n];
						if (Array.isArray(v) ? v.length > 0 : !!v) { out.candidates[n]++; }
					} catch (e) {}
				}
				try {
					if (Array.isArray(m.mentionedIds) && m.mentionedIds.length > 0) {
						out.withMentions++;
						if (!sample) { sample = m; }
					}
					if (Array.isArray(m.groupMentions) && m.groupMentions.length > 0) {
						out.withGroupMentions++;
						if (!groupSample) { groupSample = m; }
					}
				} catch (e) {}
			}
			if (sample) {
				// FORMA, NAO IDENTIDADE: quantas mencoes, de que tipo e em que
				// dominio de jid.
				const first = sample.mentionedIds[0];
				out.mentionKind = typeof first;
				out.mentionDomain = (first && first.server) ? String(first.server)
					: (typeof first === 'string' && first.indexOf('@') >= 0
						? first.split('@')[1] : 'unknown');
				out.mentionHasSerialized = !!(first && first._serialized);
				out.sampleCount = sample.mentionedIds.length;
			}
			if (groupSample) {
				const g = groupSample.groupMentions[0];
				out.groupMentionFields = (g && typeof g === 'object')
					? Object.keys(g).slice(0, 10) : typeof g;
			}
		} catch (e) { out.fatal = String((e && e.message) || e).slice(0, 130); }
		return out;
	})())`

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/mentions", func(c context.Context) error {
		return sess.Tab().Evaluate(c, script, &raw)
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Logf("%s", raw)
	if strings.Contains(raw, `"withMentions":0`) {
		t.Logf("no message in this session carries a mention; proving the reader " +
			"non-empty needs one to be sent")
	}
}
