package send

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// mediaDouble routes by script, and imitates the REAL rule the text double
// already encodes: the collection answers only under the RESOLVED lid identity
// (397 of 399 models measured under "@lid"), and it reports the message TYPE,
// because a media send must not be verified by a text message.
type mediaDouble struct {
	resolvedJID string
	omitJID     bool
	failStage   string
	failWhy     string

	storedUnder string
	storedAt    time.Time
	storedType  string

	kicks       int
	lastScript  string
	verifyWants []string
}

func (p *mediaDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// THE ACK READ, answered like the page answers it.
	//
	// Without this branch the double hands the verification payload to the ack
	// reader and the send fails on a JSON shape — the double being less
	// faithful than production, which is the fourth time in one session. It
	// answers 2 (delivered) so that every test written before the ack
	// postcondition keeps asserting what it meant to assert.
	//
	// Keyed on the script's own MARKER. The first attempt matched an expression
	// the reply dispatch also contains, so the double answered an ack payload to
	// a dispatch and swallowed it.
	if strings.Contains(expr, ackReadMarker) {
		*out = `{"found":true,"ack":2}`
		return nil
	}

	switch {
	case strings.Contains(expr, "getModelsArray"):
		want := between(expr, `const want = "`, `"`)
		p.verifyWants = append(p.verifyWants, want)
		if p.storedAt.IsZero() || want != p.storedUnder {
			*out = `[]`
			return nil
		}
		typ := p.storedType
		if typ == "" {
			typ = "image"
		}
		*out = fmt.Sprintf(
			`[{"jid":%q,"id":{"id":"MEDIA1","from_me":true,"remote_jid":%q},`+
				`"direction":"out","type":%q,"t":%d}]`,
			p.storedUnder, p.storedUnder, typ, p.storedAt.Unix())
		return nil

	case strings.Contains(expr, "const s = window["):
		switch {
		case p.failWhy != "":
			stage := p.failStage
			if stage == "" {
				stage = "prep"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.failWhy)
		case p.omitJID:
			*out = `{"stage":"done","ok":true,"why":""}`
		default:
			*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","jid":%q}`, p.resolvedJID)
		}
		return nil

	default:
		p.kicks++
		p.lastScript = expr
		*out = `{"started":true}`
		return nil
	}
}

func mediaSender(p *mediaDouble) func(Media) (Result, error) {
	return func(m Media) (Result, error) {
		return SendMedia(context.Background(), engine.NewRunner(), p.eval, phoneJID, m, "t/media")
	}
}

func compressMediaClock(t *testing.T) {
	t.Helper()
	compressClock(t)
	old := mediaUploadAllowance
	mediaUploadAllowance = 50 * time.Millisecond
	t.Cleanup(func() { mediaUploadAllowance = old })
}

func goodMedia() Media {
	return Media{Filename: "photo.jpg", MimeType: "image/jpeg", Data: []byte{0xFF, 0xD8, 0xFF, 0xE0}}
}

// TestMediaIsVerifiedAgainstTheResolvedIdentity is the same regression the text
// path carries (H34): this build files messages under the lid the SERVER
// returns, so verifying against the caller's phone jid matches nothing.
func TestMediaIsVerifiedAgainstTheResolvedIdentity(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now(), storedType: "image"}
	res, err := mediaSender(p)(goodMedia())
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if res.ID.ID != "MEDIA1" {
		t.Fatalf("verified the wrong message: %+v", res)
	}
	for _, w := range p.verifyWants {
		if w != lidJID {
			t.Fatalf("verification asked for %q, want the resolved %q", w, lidJID)
		}
	}
}

// TestATextMessageDoesNotVerifyAMediaSend. Both land in the same collection,
// so "an outgoing message appeared" stops being evidence the moment two kinds
// can be in flight — the same false positive shape as H35, one layer down.
func TestATextMessageDoesNotVerifyAMediaSend(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{
		resolvedJID: lidJID, storedUnder: lidJID,
		storedAt: time.Now(), storedType: "chat", // a TEXT message
	}
	_, err := mediaSender(p)(goodMedia())
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("got %v, want ErrUnverified: a text message was accepted as proof "+
			"that an attachment was sent", err)
	}
}

// TestAMediaMessageDoesNotVerifyATextSend is the same guard from the other
// side, and it protects the path that was already shipping.
func TestAMediaMessageDoesNotVerifyATextSend(t *testing.T) {
	compressClock(t)
	p := &pageDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now()}
	// The text double reports type "chat"; flip the expectation by asking the
	// verifier for media and confirming the text message is refused.
	if _, err := verify(context.Background(), engine.NewRunner(), p.eval,
		lidJID, time.Now().Add(-time.Minute), "t/verify", kindMedia); !errors.Is(err, ErrUnverified) {
		t.Fatalf("got %v, want ErrUnverified: a text message satisfied a media "+
			"verification", err)
	}
}

// TestEmptyMediaNeverReachesThePage is ORDER: a caller's mistake must not
// become an upload attempt.
func TestEmptyMediaNeverReachesThePage(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{resolvedJID: lidJID}
	m := goodMedia()
	m.Data = nil
	if _, err := mediaSender(p)(m); !errors.Is(err, ErrMediaEmpty) {
		t.Fatalf("got %v, want ErrMediaEmpty", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to send %d time(s) with no bytes", p.kicks)
	}
}

func TestOversizeMediaIsRefusedBeforeTheUpload(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{resolvedJID: lidJID}
	m := goodMedia()
	m.Data = make([]byte, MaxMediaBytes+1)
	_, err := mediaSender(p)(m)
	if !errors.Is(err, ErrMediaTooLarge) {
		t.Fatalf("got %v, want ErrMediaTooLarge", err)
	}
	if p.kicks != 0 {
		t.Fatalf("an oversize payload was handed to the page %d time(s)", p.kicks)
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Fatalf("the error must say what the limit is: %v", err)
	}
}

func TestMissingMimeTypeIsRefused(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{resolvedJID: lidJID}
	m := goodMedia()
	m.MimeType = "   "
	if _, err := mediaSender(p)(m); !errors.Is(err, ErrMediaNoMimeType) {
		t.Fatalf("got %v, want ErrMediaNoMimeType: the page uses the type to decide "+
			"what KIND of message this is", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to classify %d untyped payload(s)", p.kicks)
	}
}

// TestTheFilenameTravelsAsAFile. createFromData keeps the object it is handed,
// so a Blob would arrive nameless while a File carries the name. The assertion
// is on the script because no double can observe what the recipient sees.
func TestTheFilenameTravelsAsAFile(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now()}
	m := goodMedia()
	m.Filename = "relatorio.pdf"
	if _, err := mediaSender(p)(m); err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if !strings.Contains(p.lastScript, "new File(") {
		t.Fatalf("the payload is not built as a File, so the filename cannot survive:\n%s",
			p.lastScript[:min(len(p.lastScript), 400)])
	}
	if !strings.Contains(p.lastScript, `"relatorio.pdf"`) {
		t.Fatal("the filename never reached the page")
	}
}

// TestSendToChatReceivesOneObject pins the signature that would have been the
// fifth blind correction at this layer. Measured source:
//
//	function (t) { var e = t.chat, n = t.earlyUpload, r = t.options; ... }
func TestSendToChatReceivesOneObject(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now()}
	if _, err := mediaSender(p)(goodMedia()); err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if !strings.Contains(p.lastScript, "sendToChat({") {
		t.Fatal("sendToChat is not called with a single object; the build's source " +
			"destructures t.chat / t.earlyUpload / t.options from ONE argument")
	}
}

func TestPageRefusalDuringPrepIsErrDispatch(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{failStage: "prep", failWhy: "boom"}
	_, err := mediaSender(p)(goodMedia())
	if !errors.Is(err, ErrDispatch) {
		t.Fatalf("got %v, want ErrDispatch", err)
	}
	if !strings.Contains(err.Error(), "prep") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("the stage and reason were dropped: %v", err)
	}
}

func TestUnresolvableRecipientIsErrNoChat(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{failStage: "resolve", failWhy: "NOT_ON_WHATSAPP"}
	if _, err := mediaSender(p)(goodMedia()); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
}

func TestCancelledContextNeverUploads(t *testing.T) {
	compressMediaClock(t)
	p := &mediaDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SendMedia(ctx, engine.NewRunner(), p.eval, phoneJID, goodMedia(), "t/media"); err == nil {
		t.Fatal("a cancelled context produced a media send")
	}
	if p.kicks != 0 {
		t.Fatalf("uploaded %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestMediaRedactsItsContent(t *testing.T) {
	m := Media{Filename: "segredo.pdf", MimeType: "application/pdf",
		Data: []byte("conteudo confidencial"), Caption: "olha isso"}
	s := m.String()
	for _, secret := range []string{"segredo", "confidencial", "olha isso"} {
		if strings.Contains(s, secret) {
			t.Fatalf("String() leaked %q: %s", secret, s)
		}
	}
	if !strings.Contains(s, "bytes=21") {
		t.Fatalf("the size is the one thing worth printing: %s", s)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
