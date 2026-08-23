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

// TestProbeMembershipRequests measures the membership-request surface BEFORE
// anything is designed against it.
//
// The reference answered the design question — whatsapp-web.js reads through
// WAWebApiMembershipApprovalRequestStore.getMembershipApprovalRequests and
// writes through WASmaxGroupsMembershipRequestsActionRPC, one participant per
// call. It cannot answer whether those modules EXIST on this build, and that is
// the question that has been wrong four times out of four before (sendText).
//
// READ ONLY. It loads modules, prints what they export, and reads one group's
// metadata. Nothing is approved, rejected or sent.
func TestProbeMembershipRequests(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MEMBREQ") == "" {
		t.Skip("set WA_PROBE_MEMBREQ=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}

	kick := `(() => {
		window.__mr = null;
		const park = v => { window.__mr = JSON.stringify(v); };
		const shape = o => {
			if (o === null || o === undefined) return String(o);
			if (Array.isArray(o)) return 'array[' + o.length + ']';
			return typeof o;
		};
		(async () => {
		const out = { modules: {}, meta: {}, read: {} };
		const load = name => {
			try {
				const m = window.require(name);
				out.modules[name] = m ? Object.keys(m) : 'falsy';
				return m;
			} catch (e) {
				out.modules[name] = 'ABSENT: ' + (e && e.message);
				return null;
			}
		};
		try {
			const W = load('WAWebWidFactory');
			const Store = load('WAWebApiMembershipApprovalRequestStore');
			const RPC = load('WASmaxGroupsMembershipRequestsActionRPC');
			const W2J = load('WAWebWidToJid');
			const QJ = load('WAWebGroupQueryJob');
			// The round-trip proof needs conta-B to JOIN by link while approval
			// mode is on, so the module that joins is measured here too: without
			// it there is no way to create a real pending request without a
			// human and a phone.
			load('WAWebGroupInviteJob');
			const CC = window.require('WAWebChatCollection').ChatCollection;

			// The arity of the two functions we would call. A synchronous
			// String() is useless on an async function (it answers
			// apply(this, arguments)); length is not.
			if (Store && Store.getMembershipApprovalRequests) {
				out.read.arity = Store.getMembershipApprovalRequests.length;
			}
			if (RPC && RPC.sendMembershipRequestsActionRPC) {
				out.read.rpcArity = RPC.sendMembershipRequestsActionRPC.length;
			}

			const gwid = W.createWid(GROUP_PLACEHOLDER);
			out.meta.groupJid = W2J && W2J.widToGroupJid ? shape(W2J.widToGroupJid(gwid)) : 'no widToGroupJid';

			// Refresh from the server the way the reference does before reading.
			if (QJ && QJ.queryAndUpdateGroupMetadataById) {
				try { await QJ.queryAndUpdateGroupMetadataById({ id: GROUP_PLACEHOLDER }); out.meta.refreshed = true; }
				catch (e) { out.meta.refreshed = 'threw: ' + (e && e.message); }
			}

			const chat = CC.get(GROUP_PLACEHOLDER);
			const md = chat && chat.groupMetadata;
			out.meta.hasChat = !!chat;
			out.meta.hasMetadata = !!md;
			if (md) {
				out.meta.approvalMode = md.membershipApprovalMode === undefined
					? 'field absent' : md.membershipApprovalMode;
				const reqs = md.membershipApprovalRequests;
				out.meta.requestsField = shape(reqs);
				if (reqs && reqs._models) {
					out.meta.modelCount = reqs._models.length;
					// Field NAMES of one pending request, never values: a
					// requester is a phone number.
					out.meta.modelFields = reqs._models.length
						? Object.keys(reqs._models[0]) : [];
				}
			}

			// THIS BUILD HAS A MORE SPECIFIC REFRESH THAN THE REFERENCE USES.
			// whatsapp-web.js calls queryAndUpdateGroupMetadataById, which
			// refetches the WHOLE metadata; WAWebGroupQueryJob here also exports
			// maybeQueryAndUpdateMembershipApprovalRequests. Using the heavy one
			// when a targeted one exists is exactly the kind of divergence that
			// has to be a decision rather than an accident, so it is measured:
			// which argument shape does it accept?
			if (QJ && QJ.maybeQueryAndUpdateMembershipApprovalRequests) {
				const f = QJ.maybeQueryAndUpdateMembershipApprovalRequests;
				out.targeted = { arity: f.length, tries: {} };
				const attempt = async (name, arg) => {
					try { const r = await f(arg); out.targeted.tries[name] = 'resolved: ' + shape(r); }
					catch (e) { out.targeted.tries[name] = 'threw: ' + String(e && e.message); }
				};
				await attempt('wid', gwid);
				await attempt('idObject', { id: GROUP_PLACEHOLDER });
				await attempt('string', GROUP_PLACEHOLDER);
				await attempt('chat', chat);
			} else {
				out.targeted = 'absent';
			}

			if (Store && Store.getMembershipApprovalRequests) {
				try {
					const got = await Store.getMembershipApprovalRequests(gwid);
					out.read.shape = shape(got);
					out.read.count = Array.isArray(got) ? got.length : undefined;
					out.read.fields = Array.isArray(got) && got.length
						? Object.keys(got[0]) : [];
				} catch (e) {
					out.read.threw = String(e && e.message);
				}
			}
		} catch (e) {
			out.fatal = String(e && e.message);
		}
		park(out);
		})();
		return 'kicked';
	})()`

	kick = strings.ReplaceAll(kick, "GROUP_PLACEHOLDER", strconv.Quote(gjid))

	var started string
	if err := runner.Do(ctx, engine.OpStateProbe, "probe/mr-kick", func(c context.Context) error {
		return sess.Tab().Evaluate(c, kick, &started)
	}); err != nil {
		t.Fatalf("kick: %v", err)
	}
	var raw string
	for i := 0; i < 120; i++ {
		if err := runner.Do(ctx, engine.OpStateProbe, "probe/mr-read", func(c context.Context) error {
			return sess.Tab().Evaluate(c, `window.__mr || ""`, &raw)
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
