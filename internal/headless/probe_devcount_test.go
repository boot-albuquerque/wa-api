package headless

import (
	"os"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/addressbook"
	"wa-api/internal/headless/capabilities/lookup"
	"wa-api/internal/headless/capabilities/send"
)

// TestProbeDeviceCountWithPeerAwake tests the falsifiable half of
// getContactDeviceCount: whether "no device record" becomes a COUNT when the
// peer is actually awake and has just exchanged a message.
//
// H90 measured the path working and the peer carrying no device record, and
// refused to merge "no record" into the number 0 — correctly, because they are
// different answers. What it could not test is whether a record ever appears,
// since the peer was never live at the same time. The dual session (H135) is
// that missing condition.
//
// THE MESSAGE IS THE POINT, not noise: device records are established by
// exchanging encrypted traffic, so a peer that is merely awake may still be
// unknown. One message to the lab peer is the cheapest way to force the
// registration if it is going to happen at all.
func TestProbeDeviceCountWithPeerAwake(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_DEVCOUNT") == "" {
		t.Skip("set WA_PROBE_DEVCOUNT=1 (sends one message to the lab peer)")
	}
	pa, pb := os.Getenv("WA_PROFILE_A"), os.Getenv("WA_PROFILE_B")
	peerB := os.Getenv("WA_PEER_B_JID")
	if pa == "" || pb == "" || peerB == "" {
		t.Fatal("WA_PROFILE_A, WA_PROFILE_B and WA_PEER_B_JID are required")
	}
	d, ctx, done := openDual(t, pa, pb, 8*time.Minute)
	defer done()

	evalA := d.A.Tab().Evaluate
	ab := addressbook.New(d.RunnerA, evalA)
	ident, err := lookup.New(d.RunnerA, evalA).NumberID(ctx, peerB, "probe/devcount")
	if err != nil {
		t.Fatalf("resolving the peer: %v", err)
	}

	// A LINHA DE BASE, com as duas identidades. Se so' uma delas responder, o
	// numero que a linha reporta depende de qual jid o chamador tem em maos — e
	// isso e' informacao que a linha do ledger nao carrega hoje.
	for _, probe := range []struct{ what, jid string }{
		{"resolved", ident.JID}, {"asked-for", peerB},
	} {
		n, err := ab.DeviceCount(ctx, probe.jid, "probe/devcount/before-"+probe.what)
		t.Logf("BEFORE (%s): count=%d err=%v", probe.what, n, err)
	}

	if _, err := send.Text(ctx, d.RunnerA, evalA, peerB, "headless device probe", "probe/devcount"); err != nil {
		t.Fatalf("send: %v", err)
	}
	time.Sleep(8 * time.Second)

	for _, probe := range []struct{ what, jid string }{
		{"resolved", ident.JID}, {"asked-for", peerB},
	} {
		n, err := ab.DeviceCount(ctx, probe.jid, "probe/devcount/after-"+probe.what)
		t.Logf("AFTER  (%s): count=%d err=%v", probe.what, n, err)
	}
}
