package waheadless

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeContactShape is a measurement bench, not a guard. Its script is
// rewritten as questions come up; what stays is the harness.
//
// It prints counts, shapes and stacks — never a name, a number or an identity.
func TestProbeContactShape(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_CONTACTS") == "" {
		t.Skip("set WA_PROBE_CONTACTS=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_PROBE_PEER")
	if profile == "" || peer == "" {
		t.Skip("WA_SEND_FROM_PROFILE and WA_PROBE_PEER are required; the peer must be a LAB account")
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

	// THE ANSWER TO THE WHOLE CLASS: iAmAdmin is a METHOD on the participants
	// collection, not a field on the metadata.
	//
	//	chat.iAmAdmin = function(){ return this.groupMetadata
	//	    ? this.groupMetadata.participants.iAmAdmin() : false }
	//
	// So "Cannot read properties of undefined (reading 'iAmAdmin')" does not
	// mean the field is missing — it means groupMetadata.PARTICIPANTS is
	// undefined. This measures exactly that, before and after the metadata
	// query, and tries the chat's own method too.
	const script = `(() => {
		window.__waHeadlessArch = { stage: 'pending' };
		(async () => {
			const out = { stage: 'done' };
			const shape = (chat) => {
				const md = chat.groupMetadata;
				const o = { hasMetadata: !!md };
				if (md) {
					o.hasParticipants = md.participants !== undefined && md.participants !== null;
					o.participantsType = typeof md.participants;
					if (md.participants) {
						o.participantsIsCollection = typeof md.participants.getModelsArray === 'function';
						o.participantsHasIAmAdmin = typeof md.participants.iAmAdmin === 'function';
						try { o.count = md.participants.getModelsArray().length; } catch (e) {}
						try { o.iAmAdmin = md.participants.iAmAdmin(); } catch (e) { o.iAmAdminErr = String((e && e.message) || e).slice(0, 70); }
					}
				}
				try { o.chatIAmAdminIsFn = typeof chat.iAmAdmin === 'function'; } catch (e) {}
				try { o.chatIAmAdmin = (typeof chat.iAmAdmin === 'function') ? chat.iAmAdmin() : chat.iAmAdmin; }
				catch (e) { o.chatIAmAdminErr = String((e && e.message) || e).slice(0, 70); }
				return o;
			};
			try {
				const Chats = window.require('WAWebChatCollection').ChatCollection;
				let lab = null;
				for (const c of Chats.getModelsArray()) {
					try {
						if (c.id && c.id.server === 'g.us' &&
							typeof c.formattedTitle === 'string' &&
							c.formattedTitle.indexOf('wa-headless-lab') === 0) { lab = c; break; }
					} catch (e) {}
				}
				if (!lab) { out.why = 'NO_LAB_GROUP'; window.__waHeadlessArch = out; return; }

				out.before = shape(lab);
				try {
					const Job = window.require('WAWebGroupQueryJob');
					await Job.queryAndUpdateGroupMetadataById(lab.id);
					out.queried = true;
				} catch (e) { out.queryErr = String((e && e.message) || e).slice(0, 140); }
				out.after = shape(lab);

				// THE PARTICIPANTS ARE PRESENT AND iAmAdmin() RETURNS TRUE, and
				// the call still fails on the chat — so it does not read from a
				// chat. If its body is e.participants.iAmAdmin(), the argument
				// is the METADATA itself. That candidate was tried once before
				// and its result was lost to a truncated log line.
				// THE METADATA IS THE RIGHT ARGUMENT — it does not throw — and it
				// returns undefined, which means the code is not CACHED. So the
				// missing step is a fetch, and WAWebGroupQueryJob exports
				// queryGroupInvite for exactly that.
				const A = window.require('WAWebGroupInviteAction');
				const Job = window.require('WAWebGroupQueryJob');
				out.invite = {};
				out.invite.cachedBefore = (await A.queryGroupInviteCode(lab.groupMetadata)) === undefined
					? 'undefined' : 'present';

				for (const [name, arg] of [['metadata', lab.groupMetadata], ['wid', lab.id], ['chat', lab]]) {
					try {
						const r = await Job.queryGroupInvite(arg);
						out.invite['fetch_' + name] = (typeof r === 'string')
							? { len: r.length }
							: (r && typeof r === 'object' ? { keys: Object.keys(r).join(',') } : { type: typeof r });
						break;
					} catch (e) { out.invite['fetch_' + name] = 'THREW: ' + String((e && e.message) || e).slice(0, 60); }
				}
				try {
					const after = await A.queryGroupInviteCode(lab.groupMetadata);
					out.invite.cachedAfter = (typeof after === 'string') ? { len: after.length } : { type: typeof after };
				} catch (e) { out.invite.afterErr = String((e && e.message) || e).slice(0, 80); }
			} catch (e) {
				out.fatal = String((e && e.message) || e).slice(0, 200);
			}
			window.__waHeadlessArch = out;
		})();
		return 'kicked';
	})()`

	kick := script
	_ = peer
	_ = strconv.Quote

	var raw string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/arch/kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &raw)
	}); err != nil {
		t.Fatalf("probe kick: %v", err)
	}
	for i := 0; ; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/arch/poll", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `JSON.stringify(window.__waHeadlessArch || {stage:"missing"})`, &raw)
		}); err != nil {
			t.Fatalf("probe poll: %v", err)
		}
		if !strings.Contains(raw, `"stage":"pending"`) && !strings.Contains(raw, `"stage":"missing"`) {
			break
		}
		if i > 40 {
			t.Fatalf("the archive probe never settled (last: %s)", raw)
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("participants layer: %s", raw)
}
