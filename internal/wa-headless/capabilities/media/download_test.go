package media

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type pageDouble struct {
	ok         bool
	stage, why string
	body       []byte
	// filehashOverride, when set, is what the double claims the plaintext hash
	// is. It exists so a test can hand back honest bytes with a dishonest hash.
	filehashOverride string
	mime, shape      string
	size             int

	kicks      int
	lastScript string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// A LIBERACAO NAO E' KICK NEM LEITURA (H177): caindo no ramo padrao ela vira
	// lastScript e soma um kick, e todo teste que afirma sobre o script passa a
	// inspecionar o de limpeza.
	if strings.Contains(expr, "delete window.") {
		*out = "ok"
		return nil
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "download"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q,"size":%d}`, stage, p.why, p.size)
			return nil
		}
		// THE DOUBLE COMPUTES THE HASH THE WAY THE REAL PAGE WOULD — from the
		// bytes it is handing back. A double that returned a hash matching
		// whatever the caller expected would make the cryptographic check
		// untestable, which is the permissive-double trap in ARMADILHAS.md.
		sum := sha256.Sum256(p.body)
		fh := base64.StdEncoding.EncodeToString(sum[:])
		if p.filehashOverride != "" {
			fh = p.filehashOverride
		}
		shape := p.shape
		if shape == "" {
			shape = "arraybuffer"
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","b64":%q,"mime":%q,"filehash":%q,"shape":%q,"size":%d}`,
			base64.StdEncoding.EncodeToString(p.body), p.mime, fh, shape, len(p.body))
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func downloader(p *pageDouble) *Downloader { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := downloadBudget, downloadTick
	downloadBudget, downloadTick = 150*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { downloadBudget, downloadTick = ob, ot })
}

const msgID = "3EB0000000000000000000"

// TestTheHashIsVerified is the point of this package. It is the only
// postcondition in the module that no page behaviour can fake, and a version
// that skipped it would return whatever arrived.
func TestTheHashIsVerified(t *testing.T) {
	compressClock(t)
	body := []byte("the bytes that were sent")
	good := &pageDouble{ok: true, body: body, mime: "text/plain"}
	got, err := downloader(good).Get(context.Background(), msgID, "t")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	sum := sha256.Sum256(body)
	if got.SHA256 != fmt.Sprintf("%x", sum) {
		t.Fatalf("the verified hash is not the hash of the bytes: %s", got)
	}

	// Same bytes, a filehash claiming something else: the download must fail.
	other := sha256.Sum256([]byte("some other file entirely"))
	bad := &pageDouble{ok: true, body: body, mime: "text/plain",
		filehashOverride: base64.StdEncoding.EncodeToString(other[:])}
	if _, err := downloader(bad).Get(context.Background(), msgID, "t"); !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("got %v, want ErrHashMismatch", err)
	}
}

// TestTheQplHandleIsPassed. Omitting it threw
// "Cannot read properties of undefined (reading 'addAnnotations')" against the
// live build — telemetry plumbing presenting as a required argument, which is
// the least guessable kind there is.
func TestTheQplHandleIsPassed(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, body: []byte("x"), mime: "text/plain"}
	if _, err := downloader(p).Get(context.Background(), msgID, "t"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(p.lastScript, "downloadQpl: qpl") {
		t.Fatal("the download omits downloadQpl, which the live build requires")
	}
	if !strings.Contains(p.lastScript, "startMediaDownloadQpl(") {
		t.Fatal("no QPL handle is built")
	}
}

// TestAllThreeResultShapesAreHandledAndReported. What the page returns was
// measured as an ArrayBuffer on this build, but the script must not assume it —
// and it must SAY which one it got.
func TestAllThreeResultShapesAreHandledAndReported(t *testing.T) {
	compressClock(t)
	for _, shape := range []string{"arraybuffer", "typedarray", "blob"} {
		p := &pageDouble{ok: true, body: []byte("bytes"), mime: "text/plain", shape: shape}
		got, err := downloader(p).Get(context.Background(), msgID, "t")
		if err != nil {
			t.Fatalf("%s: %v", shape, err)
		}
		if got.PageShape != shape {
			t.Fatalf("the shape was not carried through: %s", got)
		}
		if !strings.Contains(p.lastScript, "shape = '"+shape+"'") {
			t.Fatalf("the script does not handle the %s shape", shape)
		}
	}
}

// TestTheCeilingIsCheckedBeforeDownloading. Fetching four megabytes in order to
// reject them wastes the sender's bandwidth as well as ours.
func TestTheCeilingIsCheckedBeforeDownloading(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: false, stage: "find", why: "TOO_LARGE", size: MaxBytes + 1}
	err := errOf(downloader(p).Get(context.Background(), msgID, "t"))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d bytes", MaxBytes+1)) {
		t.Fatalf("the error does not carry the measured size: %v", err)
	}
	// THE NEEDLE IS THE BRANCH. An earlier version asserted only the ORDER of
	// the refusal and the download, and a negative control that changed the
	// condition to `if (false && ...)` PASSED — the refusal was still in the
	// right place and no longer reachable. Position is not enforcement.
	kick := downloadScript(msgID, "k")
	if !strings.Contains(kick, "if (msg.size && Number(msg.size) > ") {
		t.Fatal("the declared size is read but does not guard a refusal")
	}
	if strings.Index(kick, "why: 'TOO_LARGE', size: Number(msg.size)") > strings.Index(kick, "downloadAndMaybeDecrypt(") {
		t.Fatal("the size refusal happens after the download")
	}
}

// TestNotMediaAndNotOnPhoneAreDistinct. One means the caller asked the wrong
// question; the other means no retry will ever work.
func TestNotMediaAndNotOnPhoneAreDistinct(t *testing.T) {
	compressClock(t)
	nm := &pageDouble{ok: false, stage: "find", why: "NOT_MEDIA"}
	if _, err := downloader(nm).Get(context.Background(), msgID, "t"); !errors.Is(err, ErrNotMedia) {
		t.Fatalf("got %v, want ErrNotMedia", err)
	}
	np := &pageDouble{ok: false, stage: "download", why: "MediaNotOnPhone: gone"}
	if _, err := downloader(np).Get(context.Background(), msgID, "t"); !errors.Is(err, ErrNotOnPhone) {
		t.Fatalf("got %v, want ErrNotOnPhone", err)
	}
	if !strings.Contains(downloadScript(msgID, "k"), "!msg.directPath || !msg.mediaKey || !msg.filehash") {
		t.Fatal("the script does not check the message carries an attachment at all")
	}
}

// TestTheMimeIsTheSendersClaim. It is reported, and the doc comment says what
// it is; sniffing would be a different capability.
func TestTheMimeIsTheSendersClaim(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, body: []byte("not really a png"), mime: "image/png"}
	got, err := downloader(p).Get(context.Background(), msgID, "t")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MIME != "image/png" {
		t.Fatalf("the sender's claim was not carried: %s", got)
	}
}

// TestNoBytesAreRendered. An attachment is content.
func TestNoBytesAreRendered(t *testing.T) {
	a := Attachment{Bytes: []byte("secret payload"), MIME: "text/plain",
		SHA256: "abcdef0123456789", PageShape: "arraybuffer"}
	s := a.String()
	if strings.Contains(s, "secret") {
		t.Fatalf("the rendering carries the payload: %s", s)
	}
	if !strings.Contains(s, "bytes=14") {
		t.Fatalf("the size is missing: %s", s)
	}
	if !strings.Contains(s, "abcdef01") || strings.Contains(s, "abcdef0123456789") {
		t.Fatalf("the hash should be abbreviated, not omitted or full: %s", s)
	}
}

func TestRefusalsCostNoPageCall(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, body: []byte("x")}
	if _, err := downloader(p).Get(context.Background(), "   ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
}

func TestBadBase64IsNotSilentlyEmpty(t *testing.T) {
	compressClock(t)
	broken := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"done","ok":true,"b64":"!!!not base64!!!","filehash":"AA==","mime":"x"}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	if _, err := New(engine.NewRunner(), broken).Get(context.Background(), msgID, "t"); err == nil {
		t.Fatal("undecodable base64 produced an empty attachment instead of an error")
	}
}

func TestCancelledContextDownloadsNothing(t *testing.T) {
	compressClock(t)
	p := &pageDouble{ok: true, body: []byte("x")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := downloader(p).Get(ctx, msgID, "t"); err == nil {
		t.Fatal("a cancelled context downloaded an attachment")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestAHungPageIsATimeout(t *testing.T) {
	compressClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	if _, err := New(engine.NewRunner(), stuck).Get(context.Background(), msgID, "t"); !errors.Is(err, ErrDownload) {
		t.Fatalf("got %v, want ErrDownload", err)
	}
}

func errOf(_ Attachment, err error) error { return err }
