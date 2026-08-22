package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/contacts"
	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeAboutBothIdentities applies H148's lesson to the next row that was
// measured before identity resolution existed.
//
// getAbout sits at PARTIAL on H70: "o par tem recado VAZIO e em cache; a busca no
// servidor não foi exercitada". H148 just showed that a reading made against the
// phone jid on a LID-first build can be a fact about the ARGUMENT rather than
// about the person — the device count read "no record" under one identity and 5
// under the other. Asking whether the same happened here costs one run.
//
// READ ONLY. Nothing is written and nobody is contacted beyond the lookup every
// send already performs.
func TestProbeAboutBothIdentities(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_ABOUT2") == "" {
		t.Skip("set WA_PROBE_ABOUT2=1")
	}
	profile := os.Getenv("WA_SEND_FROM_PROFILE")
	peer := os.Getenv("WA_SEND_TO_JID")
	if profile == "" || peer == "" {
		t.Fatal("WA_SEND_FROM_PROFILE and WA_SEND_TO_JID are required")
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
	ident, err := lookup.New(runner, eval).NumberID(ctx, peer, "probe/about2")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}
	c := contacts.New(runner, eval)
	for _, probe := range []struct{ what, jid string }{
		{"resolved", ident.JID}, {"asked-for", peer},
	} {
		// O COMPRIMENTO, NUNCA O TEXTO. O recado de alguem e' conteudo dessa
		// pessoa, e esta sonda so' precisa saber se veio algo.
		about, err := c.AboutOf(ctx, probe.jid, "probe/about2/"+probe.what)
		t.Logf("%s: %s err=%v", probe.what, about, err)
	}
}
