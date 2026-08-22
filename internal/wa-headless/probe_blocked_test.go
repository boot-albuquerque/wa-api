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

// TestProbeBlockedList asks the page whether a blocklist is reachable, instead
// of repeating the row's claim that it is not exposed.
//
// "LISTAR os bloqueados não é exposto" is a statement about OUR surface, and the
// row never said whether the PAGE offers one. Enumerating is what found
// sendDeleteMsgs (H143) after three invented names had failed, and it is cheaper
// than another round of guessing.
//
// READ ONLY. It counts and names fields; it blocks nobody and unblocks nobody.
func TestProbeBlockedList(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_BLOCKED") == "" {
		t.Skip("set WA_PROBE_BLOCKED=1")
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
	var raw string
	const script = `(() => {
		const out = {collections: {}, modules: {}, contactFlag: {}};
		try {
			const C = window.require("WAWebCollections");
			for (const k of Object.keys(C)) {
				if (/block/i.test(k)) {
					try {
						const col = C[k];
						out.collections[k] = (col && typeof col.getModelsArray === "function")
							? col.getModelsArray().length : "no getModelsArray";
					} catch (e) { out.collections[k] = "threw"; }
				}
			}
		} catch (e) { out.collectionsErr = String(e).slice(0,100); }
		for (const m of ["WAWebBlocklistCollection","WAWebBlockContactAction",
		                 "WAWebContactBlockStore","WAWebBlocklistStore"]) {
			try { const mod = window.require(m); out.modules[m] = mod ? Object.keys(mod).slice(0,8) : "empty"; }
			catch (e) { out.modules[m] = "absent"; }
		}
		// O CAMINHO DE TRAS: se a colecao de contatos marca quem esta bloqueado,
		// listar e' uma filtragem e nao precisa de colecao propria.
		try {
			const CC = window.require("WAWebContactCollection").ContactCollection;
			const all = CC.getModelsArray();
			let withFlag = 0, blocked = 0;
			for (const c of all) {
				if (typeof c.isBlocked !== "undefined") { withFlag++; if (c.isBlocked) { blocked++; } }
			}
			out.contactFlag = {contacts: all.length, withIsBlocked: withFlag, blocked: blocked};
		} catch (e) { out.contactFlag = {err: String(e).slice(0,100)}; }
		return JSON.stringify(out);
	})()`
	if err := sess.Tab().Evaluate(ctx, script, &raw); err != nil {
		t.Fatalf("eval: %v", err)
	}
	t.Logf("blocklist surface: %s", raw)
}
