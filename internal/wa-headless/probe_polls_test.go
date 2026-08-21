package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbePolls measures the poll-vote surface before anything is designed.
//
// This module can already SEND a poll (H69) and has never read a vote. The
// reference reads through a SCHEMA rather than a collection —
// WAWebPollsVotesSchema.getTable().equals(['parentMsgKey'], key) — which is a
// storage layer nothing else in this module has touched, and writes through
// WAWebPollsSendVoteMsgAction.sendVote(msg, localIdSet) with a SET of option
// local ids rather than the option names a caller has.
//
// Both are shapes worth confirming rather than copying: the schema may not exist
// here, and a Set argument crossing the boundary is the kind of thing that
// silently becomes an empty object.
//
// READ ONLY. It loads modules, reports what they export, and counts rows. No
// vote is cast.
func TestProbePolls(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_POLLS") == "" {
		t.Skip("set WA_PROBE_POLLS=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	const kick = `(() => {
		window.__polls = null;
		const park = v => { window.__polls = JSON.stringify(v); };
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, '<redacted>').slice(0, 130);
		(async () => {
		const out = { modules: {}, arity: {}, schema: {}, existing: {} };
		const load = name => {
			try {
				const m = window.require(name);
				out.modules[name] = m ? Object.keys(m).slice(0, 30) : 'falsy';
				return m;
			} catch (e) { out.modules[name] = 'ABSENT: ' + safe(e); return null; }
		};
		try {
			const Key = load('WAWebMsgKey');
			const Votes = load('WAWebPollsVotesSchema');
			const Send = load('WAWebPollsSendVoteMsgAction');
			load('WAWebPollsVoteStore');
			if (Send && Send.sendVote) out.arity.sendVote = Send.sendVote.length;
			if (Votes && Votes.getTable) {
				out.arity.getTable = Votes.getTable.length;
				try {
					const table = Votes.getTable();
					out.schema.tableKind = table ? typeof table : String(table);
					out.schema.tableKeys = table ? Object.keys(table).slice(0, 25) : [];
					out.schema.hasEquals = !!(table && typeof table.equals === 'function');
					if (table && typeof table.equals === 'function') {
						out.arity.equals = table.equals.length;
					}
				} catch (e) { out.schema.threw = safe(e); }
			}

			// IS THERE A POLL IN THIS ACCOUNT ALREADY? A read with nothing to
			// read proves the path and not the shape, and H69 sent polls from
			// here — so one may be sitting in the store.
			const MC = window.require('WAWebMsgCollection').MsgCollection;
			const all = typeof MC.getModelsArray === 'function' ? MC.getModelsArray() : [];
			let polls = 0, withOptions = 0, optionFields = [];
			for (const m of all) {
				try {
					if (m.type === 'poll_creation' || (m.pollOptions && m.pollOptions.length)) {
						polls++;
						if (m.pollOptions && m.pollOptions.length) {
							withOptions++;
							if (!optionFields.length) { optionFields = Object.keys(m.pollOptions[0]); }
						}
					}
				} catch (e) {}
			}
			out.existing.messages = all.length;
			out.existing.polls = polls;
			out.existing.withOptions = withOptions;
			// FIELD NAMES of a poll option, never the option TEXT: a poll's
			// options are content.
			out.existing.optionFields = optionFields;

			// THE TABLE'S OWN SHAPE, asked of the poll that is already here.
			// A read against a poll nobody voted on proves the path and not the
			// row shape, so the row FIELD NAMES matter more than the count.
			try {
				const Key = window.require('WAWebMsgKey');
				const Votes = window.require('WAWebPollsVotesSchema');
				let target = null;
				for (const m of all) {
					try { if (m.pollOptions && m.pollOptions.length) { target = m; break; } } catch (e) {}
				}
				if (target) {
					// THE REFERENCE CONVERTS THROUGH A STRING THIS BUILD DOES NOT
					// HAVE. MsgKey.fromString(msg.id._serialized) fails here with
					// "str is null or not a string", because _serialized is null
					// on this build — the same fact that shaped messagemeta.
					//
					// But m.id IS ALREADY A KEY. The conversion the reference
					// needs is one we can skip, which is the H34 shape again:
					// use what the page already holds instead of rebuilding it.
					out.read = {
						idKind: typeof target.id,
						serializedIsNull: target.id._serialized === null,
						hasToString: typeof target.id.toString === 'function',
						keyStr: typeof target.id.toString === 'function'
							? (target.id.toString() || '').slice(0, 4) + '...(' +
								String(target.id.toString() || '').length + ' chars)'
							: 'none',
					};
					try {
						out.read.fromStringWorks = !!Key.fromString(target.id._serialized);
					} catch (e) { out.read.fromStringWorks = 'threw: ' + safe(e); }
					const k = target.id;
					const rows = await Votes.getTable().equals(['parentMsgKey'], k.toString());
					out.read.rows = Array.isArray(rows) ? rows.length : String(rows);
					out.read.rowFields = (Array.isArray(rows) && rows.length)
						? Object.keys(rows[0]) : [];
					out.read.optionCount = target.pollOptions.length;
				} else {
					out.read = 'no poll with options in the store';
				}
			} catch (e) { out.read = 'threw: ' + safe(e); }
		} catch (e) {
			out.fatal = safe(e);
		}
		park(out);
		})();
		return 'kicked';
	})()`

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/polls-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 60; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/polls-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__polls || ""`, &raw)
		}); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if raw == "" {
		t.Fatal("the probe never settled")
	}
	t.Logf("%s", raw)
}
