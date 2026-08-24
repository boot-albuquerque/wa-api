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

// TestProbeStatusHydration asks the question that has been right three times
// today: is this zero "nobody posted", or "nobody asked"?
//
// getBroadcasts reads Status.getModelsArray and was measured at ZERO feeds. The
// row attributes that to an empty world and says proving non-empty needs THIS
// account to post — which is human-escalated (decision 61).
//
// That attribution deserves the same scrutiny that flipped mentions (H142),
// message info (H153) and reactions (H154): each was a collection read before
// anything filled it. Status feeds belong to OTHER people, so if any of 944
// contacts has an active status, a hydrated collection would show it — and a
// zero would then mean the collection needs a fetch, not that the world is empty.
//
// READ ONLY. Nothing is posted and nobody is contacted.
func TestProbeStatusHydration(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_STATUS2") == "" {
		t.Skip("set WA_PROBE_STATUS2=1")
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
	eval := sess.Tab().Evaluate
	var raw string
	const script = `(() => {
		window.__st = null;
		const safe = e => String((e && e.message) || e).replace(/\d{4,}/g,'<r>').slice(0,110);
		(async () => {
		const out = {};
		try {
			const C = window.require("WAWebCollections");
			const S = C.Status;
			out.sizeBefore = S.getModelsArray().length;
			// A PERGUNTA DIRETA. hasSynced separa "ninguem postou" de "ninguem
			// pediu" sem precisar inferir de um zero.
			try { out.hasSyncedBefore = (typeof S.hasSynced === "function") ? !!S.hasSynced() : "notFunction"; }
			catch (e) { out.hasSyncedBefore = "threw: " + safe(e); }

			if (typeof S.sync === "function") {
				try { await S.sync(); out.syncOk = true; }
				catch (e) { out.syncOk = false; out.syncErr = safe(e); }
			} else { out.syncOk = "noSync"; }

			try { out.hasSyncedAfter = (typeof S.hasSynced === "function") ? !!S.hasSynced() : "notFunction"; }
			catch (e) { out.hasSyncedAfter = "threw: " + safe(e); }
			out.sizeAfterSync = S.getModelsArray().length;

			// O outro caminho, independente da colecao.
			try {
				const A = window.require("WAWebApiStatus");
				const all = await A.getAllStatuses();
				out.getAllType = Array.isArray(all) ? ("array[" + all.length + "]") : typeof all;
			} catch (e) { out.getAllErr = safe(e); }
			out.sizeFinal = S.getModelsArray().length;
		} catch (e) { out.err = safe(e); }
		window.__st = JSON.stringify(out);
		})();
		return "kicked";
	})()`
	var ignored string
	if err := eval(ctx, script, &ignored); err != nil {
		t.Fatalf("kick: %v", err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if err := eval(ctx, "window.__st", &raw); err != nil {
			t.Fatalf("read: %v", err)
		}
		if raw != "" && raw != "null" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the status probe never answered")
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Logf("status hydration: %s", raw)
}
