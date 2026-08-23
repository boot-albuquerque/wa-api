package waheadless

import (
	"context"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/lookup"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestProbeLidAndPhoneSurface enumerates the identity-pair surface before
// anything is written against it (the H143 rule, after three invented names
// failed this week).
//
// getContactLidAndPhone is the reference's way of asking "what are BOTH
// identities of this user", and it does not read the roster: it calls
// WAWebApiContact.getCurrentLid / getPhoneNumber and falls back to
// queryWidExists to FORCE retrieval (wwebjs_util.js:1694-1716). Our contacts
// roster carries PN and LID for entries it has, which is the cache — a different
// answer for anybody the roster does not hold.
//
// This matters beyond one ledger row: H151 recorded three capabilities giving
// well-formed wrong answers to a phone jid on this LID-first build, and named
// the missing decision as "who resolves identity". This is the primitive that
// decision would be built on.
func TestProbeLidAndPhoneSurface(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_LIDPN") == "" {
		t.Skip("set WA_PROBE_LIDPN=1")
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
	rs := lookup.New(runner, sess.Tab().Evaluate)

	// DIRECAO 1: entra o telefone. O LID tem de vir da resolucao.
	fromPN, err := rs.LidAndPhone(ctx, peer, "probe/lidpn/from-pn")
	if err != nil {
		t.Fatalf("LidAndPhone(phone): %v", err)
	}
	t.Logf("from phone jid: %s", fromPN)
	if fromPN.LID == "" {
		t.Fatal("no lid came back for a peer this module resolves every day; the " +
			"reference's own helper answers {} here, which is what this replaces")
	}
	if fromPN.PN == "" {
		t.Fatal("the phone side was dropped, though it is the input")
	}
	if !fromPN.Queried {
		t.Fatal("a phone input did not report the server query it must have made")
	}

	// DIRECAO 2: entra o LID que acabou de voltar. Nao pode consultar de novo, e
	// o lado conhecido tem de ser preservado.
	fromLID, err := rs.LidAndPhone(ctx, fromPN.LID, "probe/lidpn/from-lid")
	if err != nil {
		t.Fatalf("LidAndPhone(lid): %v", err)
	}
	t.Logf("from lid: %s", fromLID)
	if fromLID.LID != fromPN.LID {
		t.Fatal("the lid handed in did not come back unchanged")
	}
	if fromLID.Queried {
		t.Fatal("a lid input cost a server round trip")
	}
	t.Logf("PROVEN: both directions answer; phone side from lid is %t "+
		"(absent is a legitimate answer, not an error)", fromLID.PN != "")
}
