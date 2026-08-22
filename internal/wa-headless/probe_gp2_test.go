package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/group"
	"wa-api/internal/wa-headless/capabilities/poll"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/events"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeGroupEvents measures what the four GROUP_* ledger rows would actually
// have to work with.
//
// The reference derives GROUP_JOIN, GROUP_LEAVE, GROUP_ADMIN_CHANGED and
// GROUP_UPDATE from a SINGLE source: messages of type "gp2", dispatched by their
// subtype. This module's ingress already listens to MsgCollection add, so those
// messages already arrive — the question is only whether the subtypes and the
// carried fields exist here, and shipping a classifier for a subtype never seen
// is the H93 trap.
//
// READ ONLY, identity-free: counts and field names.
func TestProbeGroupEvents(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_GP2") == "" {
		t.Skip("set WA_PROBE_GP2=1")
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
	eval := sess.Tab().Evaluate
	script := `(() => {
	window.__g2 = null;
	const safe = e => String((e && e.message) || e).slice(0, 140);
	try {
		const C = window.require("WAWebCollections");
		const ms = C.Msg.getModelsArray();
		const subtypes = {}, gp2Keys = {};
		let gp2 = 0;
		for (const m of ms) {
			if (m.type !== "gp2") { continue; }
			gp2++;
			const st = String(m.subtype || "<none>");
			subtypes[st] = (subtypes[st] || 0) + 1;
			for (const k of Object.keys(m)) {
				if (k.indexOf("__x_") === 0 || k === "id") { gp2Keys[k] = (gp2Keys[k]||0)+1; }
			}
		}
		// Os campos que uma GroupNotification precisa, conforme a referencia:
		// id, body, type, subtype, timestamp, author, chatId, recipientIds.
		const want = ["__x_subtype","__x_author","__x_recipients","__x_body","__x_t","__x_type"];
		const present = {};
		for (const w of want) { present[w] = gp2Keys[w] || 0; }
		// Outras fontes de evento que faltam no ledger.
		const probes = {};
		const has = (mod, fn) => { try { const m = window.require(mod); probes[mod+"."+fn] = !!(m && typeof m[fn] !== "undefined"); } catch(e){ probes[mod+"."+fn]=false; } };
		has("WAWebCollections","Msg");
		has("WAWebAppStateSyncdBootstrap","default");
		has("WAWebBatteryStore","default");
		probes["battery.WAWebBatteryStore"] = (()=>{ try { return !!window.require("WAWebBatteryStore"); } catch(e){ return false; } })();
		probes["conn.WAWebSocketModel"] = (()=>{ try { return !!window.require("WAWebSocketModel"); } catch(e){ return false; } })();
		window.__g2 = JSON.stringify({total: ms.length, gp2, subtypes, present, probes});
	} catch (e) { window.__g2 = JSON.stringify({err: safe(e)}); }
	return 'kicked';
})()
`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(45 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__g2", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never answered")
		}
		time.Sleep(400 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("group events surface:\n%s", out)
}

// TestProbeGroupEventClassification proves the reclassification against the
// LIVE bus: it drains the real session and checks that the one gp2 message this
// account holds arrives as a group event and not as a plain message.
//
// It does NOT assert that join/leave/admin arrive: producing those needs a group
// membership to actually change, and asserting on something never produced is
// what H93 warns against. What it asserts is the property that makes the whole
// classification meaningful — that a gp2 carrier is recognised at all.
func TestProbeGroupEventClassification(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_GP2") == "" {
		t.Skip("set WA_PROBE_GP2=1")
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

	hub := events.NewHub()
	var mu sync.Mutex
	counts := map[events.Type]int{}
	subtypes := map[string]int{}
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		counts[e.Type]++
		if e.Subtype != "" {
			subtypes[e.Subtype]++
		}
	})
	defer unsub()
	pump := events.NewPump(runner, sess.Tab().Evaluate, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		// Uninstall on a FRESH context: the pump context is already cancelled,
		// and a teardown that rides a dead context leaves the page handlers
		// installed — the leak this project has paid for before.
		_ = pump.Uninstall(context.Background())
	}()

	// PRODUZIR O EVENTO, em vez de esperar por um histórico.
	//
	// A única gp2 desta conta é antiga, e MC.on('add') só dispara para adições
	// NOVAS — foi isso que a primeira execução mediu: o barramento entregava
	// chat.changed e contact.changed e nenhuma gp2. Trocar o assunto do grupo de
	// laboratório emite uma gp2 de subtipo "subject", que é efeito restrito ao
	// grupo de teste e reversível (o assunto volta no defer).
	gjid := findLabGroupJID(ctx, t, runner, sess.Tab().Evaluate)
	if gjid == "" {
		t.Skip("lab group not found by subject; nothing to produce an event with")
	}
	gm := group.New(runner, sess.Tab().Evaluate)
	renamed := labGroupSubject + " (gp2 probe)"
	if _, err := gm.SetSubject(ctx, gjid, renamed, "probe/gp2"); err != nil {
		t.Fatalf("SetSubject: %v", err)
	}
	// O ASSUNTO VOLTA, e com defer registrado ANTES de qualquer coisa que possa
	// falhar — t.Cleanup rodaria depois do defer que para a sessão.
	defer func() {
		if _, err := gm.SetSubject(ctx, gjid, labGroupSubject, "probe/gp2-restore"); err != nil {
			t.Errorf("RESTORE FAILED: o grupo ficou com o assunto de teste: %v", err)
		}
	}()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		mu.Lock()
		done := counts[events.GroupUpdated] > 0
		mu.Unlock()
		if done {
			break
		}
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("event types seen: %v", counts)
	t.Logf("subtypes seen: %v", subtypes)

	// THE PROPERTY: a gp2 carrier must not arrive as a plain message.
	if counts[events.GroupUpdated] == 0 {
		t.Errorf("a group subject change produced no %s event; the gp2 carrier is "+
			"not being reclassified", events.GroupUpdated)
	} else {
		t.Logf("the gp2 carrier was reclassified as %s", events.GroupUpdated)
	}
	if counts[events.MessageAdded] > 0 && subtypes["subject"] > 0 &&
		counts[events.GroupUpdated] == 0 {
		t.Error("the subject gp2 arrived as a plain message: the classification " +
			"did not run")
	}
	if len(counts) == 0 {
		t.Error("the bus delivered nothing at all in 60s")
	}
}

// TestProbeVoteSurface asks whether a CLEAN listener exists for poll votes on
// this build, or whether the reference's approach — monkey-patching
// pollVoteTableMode.bulkUpsert — is the only door.
//
// The question matters because this repository refused page patching in H112 for
// a reason that applies here: a failed restore leaves the page altered for every
// later caller. INCOMING_CALL is the precedent in the other direction — the
// reference patched an internal Map and this build turned out to have a clean
// collection listener.
func TestProbeVoteSurface(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_VOTE") == "" {
		t.Skip("set WA_PROBE_VOTE=1")
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
	eval := sess.Tab().Evaluate
	script := `(() => {
	window.__vt = null;
	const safe = e => String((e && e.message) || e).slice(0, 140);
	const out = {};
	try {
		const C = window.require("WAWebCollections");
		const P = C.PollVote;
		out.type = typeof P;
		out.hasOn = !!(P && typeof P.on === "function");
		out.hasOff = !!(P && typeof P.off === "function");
		out.hasModels = !!(P && typeof P.getModelsArray === "function");
		out.count = (P && typeof P.getModelsArray === "function") ? P.getModelsArray().length : -1;
		out.protoKeys = P ? Object.keys(P).slice(0, 14) : [];
		if (out.count > 0) {
			const v = P.getModelsArray()[0];
			out.voteKeys = Object.keys(v).filter(k => k.indexOf("__x_")===0).slice(0,14);
		}
		// A mesma forma que o ingress ja usa para CallCollection.
		out.sameShapeAsCall = (() => {
			try { const CC = C.Call; return !!(CC && typeof CC.on === "function"); } catch(e){ return false; }
		})();
	} catch (e) { out.err = safe(e); }
	window.__vt = JSON.stringify(out);
	return 'kicked';
})()
`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__vt", &raw); err != nil {
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
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("vote surface:\n%s", out)
}

// TestProbeVoteEventLive tries to PRODUCE a poll vote and see it on the bus.
//
// The listener is installed on a collection measured EMPTY, which is the
// CallIncoming position — installed, never seen firing. The only way out of that
// position is to make the thing happen, which is what the group subject change
// did for GROUP_UPDATE (H119).
//
// It votes on a poll ALREADY IN THIS ACCOUNT's store. If there is none, it says
// so and skips rather than sending a poll — H98 measured that poll SEND does not
// leave this build, so creating one to vote on would be building on sand.
func TestProbeVoteEventLive(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_VOTE_LIVE") == "" {
		t.Skip("set WA_PROBE_VOTE_LIVE=1 (this VOTES on a poll)")
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
	eval := sess.Tab().Evaluate

	// Find a poll and ONE of its options, inside the page. The option text is
	// poll content, so only its INDEX and length come back.
	const findPoll = `(() => {
		window.__pp = null;
		try {
			const ms = window.require("WAWebCollections").Msg.getModelsArray();
			for (const m of ms) {
				if (m.type !== "poll_creation") { continue; }
				const opts = m.pollOptions || m.__x_pollOptions || [];
				if (!opts.length) { continue; }
				const first = opts[0];
				const name = (typeof first === "string") ? first : (first && (first.name || first.optionName));
				if (!name) { continue; }
				window.__pollId = m.id && m.id.id;
				window.__pollOpt = name;
				window.__pp = JSON.stringify({found: true, options: opts.length, optLen: String(name).length,
					fromMe: !!(m.id && m.id.fromMe)});
				return "kicked";
			}
			window.__pp = JSON.stringify({found: false});
		} catch (e) { window.__pp = JSON.stringify({found: false, err: String(e).slice(0,120)}); }
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, findPoll, &ignored); err != nil {
		t.Fatalf("find poll: %v", err)
	}
	var found string
	for i := 0; i < 40; i++ {
		if err := eval(ctx, "window.__pp", &found); err != nil {
			t.Fatalf("read: %v", err)
		}
		if found != "" && found != "null" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Logf("poll search: %s", found)
	if !strings.Contains(found, `"found":true`) {
		t.Skip("no poll with readable options in this store")
	}
	var pollID, opt string
	if err := eval(ctx, "window.__pollId", &pollID); err != nil {
		t.Fatalf("poll id: %v", err)
	}
	if err := eval(ctx, "window.__pollOpt", &opt); err != nil {
		t.Fatalf("poll option: %v", err)
	}

	hub := events.NewHub()
	var mu sync.Mutex
	counts := map[events.Type]int{}
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		counts[e.Type]++
	})
	defer unsub()
	pump := events.NewPump(runner, eval, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()
	time.Sleep(6 * time.Second)

	pm := poll.New(runner, eval)
	voteErr := pm.Vote(ctx, pollID, []string{opt}, "probe/vote")
	t.Logf("Vote returned: %v", voteErr)

	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		mu.Lock()
		n := counts[events.VoteUpdated]
		mu.Unlock()
		if n > 0 {
			break
		}
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("types seen: %v", counts)
	if counts[events.VoteUpdated] == 0 {
		t.Logf("MEASURED: no %s arrived. The listener is installed on a clean door "+
			"and the collection stayed empty — the CallIncoming position, reported "+
			"and not asserted away.", events.VoteUpdated)
	} else {
		t.Logf("the vote listener FIRED: %d %s", counts[events.VoteUpdated], events.VoteUpdated)
	}
}
