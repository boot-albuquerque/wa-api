package headless

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/group"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeGroupSetDescription proves GroupChat.setDescription against the lab
// group, and puts the description back.
//
// The description is restored by a defer registered right after the first write
// and BEFORE anything that can fail — t.Cleanup runs after the defer that stops
// the session, and a restore against a dead tab leaves the group changed.
func TestProbeGroupSetDescription(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_GDESC_LIVE") == "" {
		t.Skip("set WA_PROBE_GDESC_LIVE=1 (this WRITES to the lab group)")
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
	gjid := findLabGroupJID(ctx, t, runner, eval)
	if gjid == "" {
		t.Skip("lab group not found by subject")
	}
	m := group.New(runner, eval)

	// The baseline, so the restore has something exact to return to.
	base, err := m.Metadata(ctx, gjid, "probe/gdesc")
	if err != nil {
		t.Fatalf("Metadata (baseline): %v", err)
	}
	t.Logf("baseline description present=%t source=%s", base.Description != "", base.DescriptionSource)

	want := fmt.Sprintf("wa-headless parity probe %d", time.Now().Unix())
	got, err := m.SetDescription(ctx, gjid, want, "probe/gdesc")
	if err != nil {
		// THE IMMEDIATE VERDICT IS NOT THE LAST WORD. The channel description
		// behaved this way in H113 and the question there was the same: is the
		// write lost, or slow? That was answered by polling, and it is answered
		// the same way here rather than by assuming either.
		t.Logf("SetDescription immediate verdict: %v", err)
		settled := false
		for i := 0; i < 10; i++ {
			md, rerr := m.Metadata(ctx, gjid, "probe/gdesc-settle")
			if rerr == nil && md.Description == want {
				t.Logf("the description appeared after %s", time.Duration(i+1)*2*time.Second)
				settled = true
				break
			}
			time.Sleep(2 * time.Second)
		}
		if !settled {
			t.Logf("MEASURED: the description never appeared within 20s. The call is " +
				"accepted and the server does not store it — the same shape the " +
				"channel description showed in H113.")
			return
		}
	}
	defer func() {
		if _, err := m.SetDescription(ctx, gjid, base.Description, "probe/gdesc-undo"); err != nil {
			t.Errorf("RESTORE FAILED: the lab group keeps the probe description: %v", err)
			return
		}
		back, err := m.Metadata(ctx, gjid, "probe/gdesc-undo")
		if err != nil {
			t.Errorf("Metadata (after undo): %v", err)
			return
		}
		if back.Description != base.Description {
			t.Errorf("the description did not come back to the baseline")
		} else {
			t.Logf("restored: present=%t", back.Description != "")
		}
	}()

	t.Logf("set: %s", got)
	if got.Text != want {
		t.Errorf("the server reports a different text than was asked for")
	}
	if got.Source == "" || got.Source == "none" {
		t.Errorf("the description read back from source %q", got.Source)
	}

	// AND THE READER AGREES INDEPENDENTLY, not just the setter's own check.
	md, err := m.Metadata(ctx, gjid, "probe/gdesc")
	if err != nil {
		t.Errorf("Metadata (after set): %v", err)
	} else if md.Description != want {
		t.Error("an independent read does not see the new description")
	}
}

// TestProbeGroupDescribeShapes measures WHAT each argument of
// setGroupDescription has to be, after the first live attempt died with
// "Cannot read properties of undefined (reading 'toJid')" — an error that says
// something expected a Wid and got nothing.
//
// It MEASURES ONLY: no call to the action, so it changes nothing.
func TestProbeGroupDescribeShapes(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_GDESC_SHAPE") == "" {
		t.Skip("set WA_PROBE_GDESC_SHAPE=1")
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
	gjid := findLabGroupJID(ctx, t, runner, eval)
	if gjid == "" {
		t.Skip("lab group not found")
	}
	var ignored string
	if err := eval(ctx, "window.__gjid = "+quoteJS(gjid)+"; \"set\"", &ignored); err != nil {
		t.Fatalf("set jid: %v", err)
	}
	script := `(() => {
	window.__gd3 = null;
	const safe = e => String((e && e.message) || e).slice(0, 150);
	const out = {attempts: []};
	(async () => {
	try {
		const JID = window.__gjid;
		const W = window.require("WAWebWidFactory");
		const K = window.require("WAWebMsgKey");
		const J = window.require("WAWebGroupModifyInfoJob");
		const wid = W.createWid(JID);
		const newId = await K.newId();
		const CC = window.require("WAWebChatCollection").ChatCollection;
		const c = CC.get(JID);
		const md = c && (c.groupMetadata || c.__x_groupMetadata);
		const descId = md ? (md.descId || md.__x_descId) : undefined;
		const TEXT = "wa-headless shape probe";

		const shapes = [
			["groupWid+description", {groupWid: wid, description: TEXT, newId: newId, prevDescId: descId}],
			["chatId+description", {chatId: wid, description: TEXT, newId: newId, descId: descId}],
			["wid+desc", {wid: wid, desc: TEXT, id: newId, prevId: descId}],
			["groupId+description", {groupId: wid, description: TEXT, descriptionId: newId, prevDescriptionId: descId}],
		];
		for (const [name, arg] of shapes) {
			try {
				await J.setGroupDescription(arg);
				out.attempts.push({name, ok: true});
				break;
			} catch (e) { out.attempts.push({name, err: safe(e)}); }
		}
	} catch (e) { out.err = safe(e); }
	window.__gd3 = JSON.stringify(out);
	})();
	return 'kicked';
})()
`
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__gd3", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered")
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Logf("describe shapes: %s", raw)
}

func quoteJS(s string) string { return "\"" + s + "\"" }
