package waheadless

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/phone"
	"wa-api/internal/wa-headless/capabilities/settings"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
	"wa-api/internal/wa-headless/spa"
)

// TestProbeSettingsSurface measures whether the modules five Client rows depend
// on EXIST in this build, and what they currently answer.
//
// The measurement is not ceremony: copying the reference's module list has
// already failed four times out of four in this repository (CLAUDE.md, sendText
// / H34). The reference names WAWebUserPrefsGeneral, WAWebUserPrefsNotifications,
// WAWebPhoneUtils, WAPhoneFindCC and window.Debug.VERSION; whether this build
// has them is a question, not an assumption.
//
// READ ONLY. Nothing is written; the current values are read so a later write
// can restore them.
func TestProbeSettingsSurface(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SETTINGS") == "" {
		t.Skip("set WA_PROBE_SETTINGS=1")
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

	const script = `(() => {
		window.__st = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 120);
		const out = {modules: {}, values: {}, funcs: {}};
		const load = name => {
			try { const m = window.require(name); out.modules[name] = !!m; return m; }
			catch (e) { out.modules[name] = false; return null; }
		};
		const G = load("WAWebUserPrefsGeneral");
		const N = load("WAWebUserPrefsNotifications");
		const P = load("WAWebPhoneUtils");
		const CC = load("WAPhoneFindCC");
		// Os nomes das funcoes importam tanto quanto o do modulo: um modulo que
		// existe com outra API falha igual, so' que mais tarde.
		for (const [mod, names] of [[G, ["getAutoDownloadAudio","setAutoDownloadAudio",
				"getAutoDownloadDocuments","getAutoDownloadPhotos","getAutoDownloadVideos"]],
			[N, ["getGlobalOfflineNotifications","setGlobalOfflineNotifications"]],
			[P, ["formattedPhoneNumber"]], [CC, ["findCC"]]]) {
			if (!mod) { continue; }
			for (const n of names) { out.funcs[n] = (typeof mod[n] === "function"); }
		}
		try {
			out.values.audio = G ? G.getAutoDownloadAudio() : null;
			out.values.documents = G ? G.getAutoDownloadDocuments() : null;
			out.values.photos = G ? G.getAutoDownloadPhotos() : null;
			out.values.videos = G ? G.getAutoDownloadVideos() : null;
		} catch (e) { out.values.err = safe(e); }
		try {
			out.values.backgroundSync = N ? N.getGlobalOfflineNotifications() : null;
		} catch (e) { out.values.syncErr = safe(e); }
		try {
			out.debugVersion = (window.Debug && typeof window.Debug.VERSION === "string")
				? "present" : "absent";
		} catch (e) { out.debugVersion = "threw"; }
		window.__st = JSON.stringify(out);
		return 'kicked';
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__st", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the page never answered")
		}
		time.Sleep(200 * time.Millisecond)
	}
	var pretty map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("settings surface:\n%s", out)
}

// TestProbeSettingsRoundTrip WRITES to the lab account and puts every value back.
//
// The restore is registered with defer and NOT with t.Cleanup, because
// t.Cleanup runs after every defer — including the one that stops the session —
// and a restore that runs against a dead tab fails with "context canceled" and
// leaves the account changed. That cost a run once already this project.
func TestProbeSettingsRoundTrip(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_SETTINGS_WRITE") == "" {
		t.Skip("set WA_PROBE_SETTINGS_WRITE=1 (this WRITES to the account)")
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
	m := settings.New(runner, sess.Tab().Evaluate)

	base, err := m.Read(ctx, "probe/settings")
	if err != nil {
		t.Fatalf("baseline read: %v", err)
	}
	t.Logf("baseline: %v sync=%t", base.AutoDownload, base.BackgroundSync)

	// THE BASELINE IS RESTORED WHATEVER HAPPENS, and the restore is itself
	// verified — an unverified restore is the failure that poisons the NEXT run.
	defer func() {
		for kind, want := range base.AutoDownload {
			if _, err := m.SetAutoDownload(ctx, kind, want, "probe/settings-restore"); err != nil {
				t.Errorf("RESTORE FAILED for %s: %v — the account is left changed", kind, err)
			}
		}
		if _, err := m.SetBackgroundSync(ctx, base.BackgroundSync, "probe/settings-restore"); err != nil {
			t.Errorf("RESTORE FAILED for backgroundSync: %v", err)
		}
		after, err := m.Read(ctx, "probe/settings-restore")
		if err != nil {
			t.Errorf("restore verification: %v", err)
			return
		}
		for kind, want := range base.AutoDownload {
			if after.AutoDownload[kind] != want {
				t.Errorf("%s did not come back to %t", kind, want)
			}
		}
		if after.BackgroundSync != base.BackgroundSync {
			t.Errorf("backgroundSync did not come back to %t", base.BackgroundSync)
		}
		t.Logf("restored: %v sync=%t", after.AutoDownload, after.BackgroundSync)
	}()

	// FLIP EVERY CATEGORY, so a setter wired to the wrong category shows up as a
	// value that moved in the wrong place rather than as a passing test.
	for _, kind := range settings.Kinds {
		want := !base.AutoDownload[kind]
		got, err := m.SetAutoDownload(ctx, kind, want, "probe/settings")
		if err != nil {
			t.Errorf("SetAutoDownload(%s, %t): %v", kind, want, err)
			continue
		}
		if !got.Changed {
			t.Errorf("%s reported no change while being flipped from %t", kind, base.AutoDownload[kind])
		}
		t.Logf("%s: %t -> %s", kind, base.AutoDownload[kind], got)
	}

	mid, err := m.Read(ctx, "probe/settings")
	if err != nil {
		t.Fatalf("read after flips: %v", err)
	}
	for kind, was := range base.AutoDownload {
		if mid.AutoDownload[kind] == was {
			t.Errorf("%s is still %t after being flipped: the write did not reach the "+
				"category it named", kind, was)
		}
	}

	// A REDUNDANT WRITE, against the real page: it must report no change.
	again, err := m.SetAutoDownload(ctx, settings.KindAudio, mid.AutoDownload[settings.KindAudio], "probe/settings")
	if err != nil {
		t.Errorf("redundant write: %v", err)
	} else if again.Changed {
		t.Error("a redundant write reported a change against the real page")
	} else {
		t.Logf("redundant write: %s", again)
	}

	sync, err := m.SetBackgroundSync(ctx, !base.BackgroundSync, "probe/settings")
	if err != nil {
		t.Errorf("SetBackgroundSync: %v", err)
	} else {
		t.Logf("backgroundSync: %t -> %s", base.BackgroundSync, sync)
	}
}

// TestProbePhoneShapes measures what formattedPhoneNumber and findCC ANSWER on
// this build, for the two lab numbers and for inputs that should not resolve.
//
// PII: the formatted number IS a phone number, so nothing here logs a value.
// Only shapes travel: length, whether it starts with "+", whether it differs
// from the input, and the country code — which names a country of 200 million
// people and identifies nobody.
func TestProbePhoneShapes(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PHONE") == "" {
		t.Skip("set WA_PROBE_PHONE=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	peer := strings.TrimSpace(os.Getenv("WA_SEND_TO_JID"))
	if peer == "" {
		t.Skip("WA_SEND_TO_JID is required")
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

	// The inputs are passed as a JSON array so the script never concatenates a
	// number into source, and the report is built INSIDE the page so no value
	// crosses into Go.
	inputs, _ := json.Marshal([]string{peer, "notaphone", "", "1"})
	script := `(() => {
		window.__ph = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g, "<redacted>").slice(0, 100);
		const P = window.require("WAWebPhoneUtils");
		const CC = window.require("WAPhoneFindCC");
		const out = [];
		for (const raw of ` + string(inputs) + `) {
			const row = {inputLen: raw.length, hadSuffix: raw.indexOf("@") >= 0};
			// A referencia normaliza para @s.whatsapp.net antes de perguntar.
			let n = raw;
			if (!n.endsWith("@s.whatsapp.net")) { n = n.replace("c.us", "s.whatsapp.net"); }
			if (n.indexOf("@s.whatsapp.net") < 0) { n = n + "@s.whatsapp.net"; }
			try {
				const f = P.formattedPhoneNumber(n);
				row.formatted = (typeof f === "string")
					? {len: f.length, plus: f.charAt(0) === "+", spaces: (f.match(/ /g) || []).length,
					   differsFromInput: f !== raw}
					: {type: typeof f};
			} catch (e) { row.formattedErr = safe(e); }
			try {
				const c = CC.findCC(raw.replace(" ", "").replace("+", "").replace("@c.us", ""));
				row.cc = (typeof c === "string") ? c : {type: typeof c, value: String(c)};
			} catch (e) { row.ccErr = safe(e); }
			out.push(row);
		}
		window.__ph = JSON.stringify(out);
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	var raw string
	for {
		if err := eval(ctx, "window.__ph", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the page never answered")
		}
		time.Sleep(200 * time.Millisecond)
	}
	var pretty []map[string]any
	if err := json.Unmarshal([]byte(raw), &pretty); err != nil {
		t.Fatalf("payload: %v", err)
	}
	out, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("phone shapes (labels only, no values):\n%s", out)
}

// TestProbePhoneCapability proves capabilities/phone against the real page, and
// specifically proves that it refuses what the page would have answered.
//
// PII: no value is logged. Shapes only.
func TestProbePhoneCapability(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_PHONE") == "" {
		t.Skip("set WA_PROBE_PHONE=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	if profile == "" {
		t.Fatal("WA_SEND_FROM_PROFILE is required")
	}
	peer := strings.TrimSpace(os.Getenv("WA_SEND_TO_JID"))
	if peer == "" {
		t.Skip("WA_SEND_TO_JID is required")
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
	r := phone.New(runner, sess.Tab().Evaluate)

	got, err := r.Lookup(ctx, peer, "probe/phone")
	if err != nil {
		t.Fatalf("Lookup(real jid): %v", err)
	}
	t.Logf("real jid -> %s", got) // String() carries shape only
	if got.CountryCode == "" {
		t.Error("a real number produced no country code")
	}
	if got.Formatted == "" {
		t.Error("a real number produced no rendering")
	}

	// THE POINT OF THE PACKAGE: the page answers junk, and this refuses it.
	// Without the guard, findCC("notaphone") returns "not" and a caller believes
	// it — measured on this build in TestProbePhoneShapes.
	for _, junk := range []string{"notaphone", "hello", "e10"} {
		if n, err := r.Lookup(ctx, junk, "probe/phone"); err == nil {
			t.Errorf("junk was accepted and answered %s — the page would have said "+
				"something country-code shaped and this did not stop it", n)
		}
	}
	t.Log("junk refused on all three inputs")

	// The web version, read from the same session. It is a build identifier, not
	// account data, so it is safe to log in full — and it is the one thing worth
	// having next to a module-inventory failure.
	v, err := spa.WebVersion(ctx, runner, sess.Tab().Evaluate)
	if err != nil {
		t.Errorf("WebVersion: %v", err)
	} else {
		t.Logf("web version: %s", v)
	}
}
