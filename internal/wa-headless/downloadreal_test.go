package waheadless

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"wa-api/internal/wa-headless/capabilities/media"
	"wa-api/internal/wa-headless/capabilities/send"
	"wa-api/internal/wa-headless/core"
	"wa-api/internal/wa-headless/engine"
	waruntime "wa-api/internal/wa-headless/runtime"
)

// TestRealSPADownloadsWhatItJustSent closes the loop the strongest way this
// module can: it sends bytes it generated, downloads them back, and compares
// the two byte for byte.
//
// The capability already checks the message's filehash, which is cryptographic
// on its own. This test adds the half the capability cannot do: it knows what
// the plaintext was SUPPOSED to be. A page that returned a different file whose
// filehash happened to match its own contents would pass the capability's check
// and fail this one.
func TestRealSPADownloadsWhatItJustSent(t *testing.T) {
	requireRealSPA(t)
	if os.Getenv("WA_HEADLESS_MEDIA_TEST") == "" {
		t.Skip("set WA_HEADLESS_MEDIA_TEST=1; this sends a small file to the peer lab account")
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	sess, err := h.Session(ctx)
	if err != nil {
		t.Fatalf("boot: %v", err)
	}

	// A small file whose bytes are known here and nowhere else, so a match
	// cannot be a coincidence of fixtures.
	body := []byte(fmt.Sprintf("wa-headless download probe %d\n", time.Now().UnixNano()))
	for len(body) < 1024 {
		body = append(body, byte(len(body)%251))
	}
	want := sha256.Sum256(body)

	sent, err := send.SendMedia(ctx, runner, sess.Tab().Evaluate, peer, send.Media{
		Filename:   "wa-headless-download-probe.bin",
		MimeType:   "application/octet-stream",
		Data:       body,
		AsDocument: true,
	}, "test/dl-send")
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	t.Logf("sent: %s", sent)

	d := media.New(runner, sess.Tab().Evaluate)
	got, err := d.Get(ctx, sent.ID.ID, "test/dl")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	t.Logf("downloaded: %s", got)
	t.Logf("MEASURED: on this build downloadAndMaybeDecrypt returns a %s", got.PageShape)

	if len(got.Bytes) != len(body) {
		t.Fatalf("got %d bytes, sent %d", len(got.Bytes), len(body))
	}
	if got.SHA256 != fmt.Sprintf("%x", want) {
		t.Fatalf("the round trip changed the bytes: got %s, sent %x", got.SHA256, want)
	}
	for i := range body {
		if got.Bytes[i] != body[i] {
			t.Fatalf("byte %d differs: got %d, sent %d", i, got.Bytes[i], body[i])
		}
	}

	// A TEXT MESSAGE HAS NOTHING TO DOWNLOAD, and saying so is more useful than
	// a generic failure.
	txt, err := send.Text(ctx, runner, sess.Tab().Evaluate, peer,
		fmt.Sprintf("wa-headless not-media probe %d", time.Now().UnixNano()), "test/dl-text")
	if err != nil {
		t.Fatalf("send.Text: %v", err)
	}
	if _, err := d.Get(ctx, txt.ID.ID, "test/dl-not-media"); err == nil {
		t.Fatal("downloading a text message succeeded")
	} else {
		t.Logf("refused a text message: %v", err)
	}
}
