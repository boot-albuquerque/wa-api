package headless

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"wa-api/internal/headless/capabilities/send"
	"wa-api/internal/headless/core"
	"wa-api/internal/headless/engine"
	"wa-api/internal/headless/events"
	waruntime "wa-api/internal/headless/runtime"
)

// TestProbeMediaUploadedEvent answers the question the ledger left open on
// MEDIA_UPLOADED: does sending media produce anything on this bus, and does
// message.added carrying Kind already cover the semantics?
//
// The row's own note says `send.SendMedia` o dispararia, e `message.added` com
// `Kind` PODE já cobrir a semântica — mas isso não foi medido, e "provavelmente
// coberto" não é um estado deste vocabulário (H88). This measures it.
//
// It SENDS to the lab peer, which is an outward effect confined to the two lab
// accounts. Identity-free: only event types, kinds and counts are logged.
func TestProbeMediaUploadedEvent(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_PROBE_MEDIAEV") == "" {
		t.Skip("set WA_PROBE_MEDIAEV=1 (this SENDS a message)")
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
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	eval := sess.Tab().Evaluate

	hub := events.NewHub()
	var mu sync.Mutex
	type row struct {
		Type events.Type
		Kind string
	}
	var seen []row
	unsub := hub.Subscribe(func(e events.Event) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, row{e.Type, e.Kind})
	})
	defer unsub()

	pump := events.NewPump(runner, eval, hub)
	pumpCtx, stopPump := context.WithCancel(ctx)
	go func() { _ = pump.Run(pumpCtx) }()
	defer func() {
		stopPump()
		_ = pump.Uninstall(context.Background())
	}()

	// Let hydration drain first, so the sample after the send is attributable.
	time.Sleep(8 * time.Second)
	mu.Lock()
	before := len(seen)
	mu.Unlock()

	// A 1x1 PNG — the smallest real image, so the upload is genuine without
	// pushing bytes at the lab peer.
	png, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("png: %v", err)
	}
	res, err := send.SendMedia(ctx, runner, eval, peer, send.Media{
		Filename: "probe.png", MimeType: "image/png", Data: png,
		Caption: fmt.Sprintf("headless media event probe %d", time.Now().Unix()),
	}, "probe/mediaev")
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	t.Logf("sent: ack=%d", res.Ack)

	// Watch for what the send produced.
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n > before {
			// Keep watching a little past the first arrival: the upload event, if
			// there is one, may follow the add.
			time.Sleep(6 * time.Second)
			break
		}
	}

	mu.Lock()
	defer mu.Unlock()
	after := seen[before:]
	kinds := map[string]int{}
	types := map[events.Type]int{}
	for _, r := range after {
		types[r.Type]++
		if r.Kind != "" {
			kinds[r.Kind]++
		}
	}
	t.Logf("after the send: %d events; types=%v kinds=%v", len(after), types, kinds)

	// THE QUESTION, answered rather than assumed: did a message.added carrying
	// an image kind arrive?
	if kinds["image"] == 0 {
		t.Logf("MEASURED: no event carried kind=image. message.added does NOT cover " +
			"MEDIA_UPLOADED's semantics on this build.")
	} else {
		t.Logf("MEASURED: message.added arrived with kind=image %d time(s) — the "+
			"semantics ARE reachable, as a kind rather than as a distinct type",
			kinds["image"])
	}
	if len(after) == 0 {
		t.Error("the send produced no events at all, which would mean the bus misses " +
			"this account's own outgoing media")
	}
}
