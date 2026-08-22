package waheadless

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeNotRegistered exercises the answer nobody had ever seen: NO.
//
// `getNumberId` is PROVEN (H127) and `isRegisteredUser` — which the reference
// defines as Boolean(await getNumberId(id)) — sits at PARTIAL with the note "é
// passo interno de todo envio; não exposto". Half of that note is stale: the
// lookup package exposed it. The other half is real and was never named: every
// live proof of this resolution asked about a number that EXISTS. A capability
// only ever seen saying yes is not proven, because "yes to everything" passes
// every test that only asks about real numbers.
//
// THE QUERY CONTACTS NOBODY. queryWidExists is the same lookup every send
// performs before dispatching; no message is sent and no conversation is opened.
// The number used is syntactically valid and deliberately implausible, so no real
// person is implicated by the question.
func TestProbeNotRegistered(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_NOTREG") == "" {
		t.Skip("set WA_PROBE_NOTREG=1")
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	res := lookup.New(runner, sess.Tab().Evaluate)

	// O CONTROLE POSITIVO VEM PRIMEIRO. Um "nao existe" so' significa alguma
	// coisa se a mesma chamada, na mesma sessao, tiver acabado de dizer "existe"
	// sobre alguem — senao o zero e' compativel com a resolucao inteira estar
	// quebrada. Foi essa a licao da H114 e ela vale aqui inteira.
	peer := os.Getenv("WA_SEND_TO_JID")
	if peer == "" {
		t.Fatal("WA_SEND_TO_JID is required as the positive control")
	}
	yes, err := res.NumberID(ctx, peer, "probe/notreg/control")
	if err != nil {
		t.Fatalf("the positive control failed, so a negative would prove nothing: %v", err)
	}
	t.Logf("positive control: %s", yes)

	// Numero sintaticamente valido e deliberadamente implausivel.
	const absent = "5599999999999@c.us"
	no, err := res.NumberID(ctx, absent, "probe/notreg/absent")
	t.Logf("implausible number: identity=%s err=%v", no, err)
	if err == nil {
		t.Fatalf("the resolution answered YES for an implausible number; it is not " +
			"distinguishing registered from unregistered, which is what " +
			"isRegisteredUser is entirely made of")
	}
	if !errors.Is(err, lookup.ErrNotOnWhatsApp) {
		t.Fatalf("the negative came back as %v rather than ErrNotOnWhatsApp; the "+
			"caller cannot tell 'not registered' from 'the lookup broke'", err)
	}
	t.Log("PROVEN: the resolution answers NO, and says so in its own error")
}
