package core

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/headless/spa"
)

// TestStartPairingSession_QRPage_Succeeds: the exact page
// TestStartSession_QRPage_TerminatesFastWithSpecificCause proves StartSession
// refuses (qrPage, ClassLoginRequired) is the page StartPairingSession exists
// to accept — HOUSEKEEP H145: nothing in this repository could boot into a
// QR-showing page before this function.
func TestStartPairingSession_QRPage_Succeeds(t *testing.T) {
	cfg := baseConfig(t, pageServer(t, "/qr-pairing", qrPage))

	sess, err := StartPairingSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartPairingSession on a QR page: %v", err)
	}
	defer sess.Stop(context.Background())
}

// TestStartPairingSession_ReadyPage_StillSucceeds: a profile that turns out
// to already be paired is a FINE outcome for the pairing-tolerant boot too,
// not just a fallback — see StartPairingSession's own doc comment.
func TestStartPairingSession_ReadyPage_StillSucceeds(t *testing.T) {
	cfg := baseConfig(t, pageServer(t, "/ready-pairing", readyPage))

	sess, err := StartPairingSession(context.Background(), cfg)
	if err != nil {
		t.Fatalf("StartPairingSession on an already-ready page: %v", err)
	}
	defer sess.Stop(context.Background())
}

// TestStartSession_UnaffectedByAcceptClasses: StartSession itself (cfg.
// AcceptClasses left nil) must still refuse the QR page exactly as before —
// StartPairingSession is additive, not a relaxation of the restoration-only
// contract. Redundant with TestStartSession_QRPage_TerminatesFastWithSpecificCause
// by design: that test already covers this from before AcceptClasses existed,
// and this one pins it explicitly as a regression guard for THIS change.
func TestStartSession_UnaffectedByAcceptClasses(t *testing.T) {
	cfg := baseConfig(t, pageServer(t, "/qr-unaffected", qrPage))

	_, err := StartSession(context.Background(), cfg)
	if err == nil {
		t.Fatal("StartSession succeeded on a QR page — AcceptClasses default must still be ClassAppReady-only")
	}
	var boot *BootFailure
	if !errors.As(err, &boot) || boot.Stage != StageNotReady {
		t.Fatalf("error = %v, want a StageNotReady *BootFailure", err)
	}
}

// TestAcceptsClass_EmptyDefaultsToAppReadyOnly locks acceptsClass's own
// contract directly, without paying for a browser boot.
func TestAcceptsClass_EmptyDefaultsToAppReadyOnly(t *testing.T) {
	if !acceptsClass(spa.ClassAppReady, nil) {
		t.Error("nil accepted list must still accept ClassAppReady")
	}
	if acceptsClass(spa.ClassLoginRequired, nil) {
		t.Error("nil accepted list must NOT accept ClassLoginRequired — that is the restoration-only contract")
	}
	custom := []spa.PageClass{spa.ClassLoginRequired, spa.ClassPairingLoading}
	if acceptsClass(spa.ClassAppReady, custom) {
		t.Error("an explicit accepted list must not silently also accept ClassAppReady")
	}
	if !acceptsClass(spa.ClassLoginRequired, custom) || !acceptsClass(spa.ClassPairingLoading, custom) {
		t.Error("an explicit accepted list must accept every class it names")
	}
}
