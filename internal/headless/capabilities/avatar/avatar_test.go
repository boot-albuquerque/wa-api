package avatar

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

// pageDouble imitates the REAL shape measured on 2026-08-20 over twelve live
// contacts, and the important half is the second row: a person with no picture
// comes back as a NORMAL result whose url fields are absent — not as null, not
// as a throw. A double that returned an error there would be more hostile than
// the world and would let the capability's central contract go untested.
type pageDouble struct {
	// present drives which of the two measured shapes is returned.
	present bool
	// nullResult and failWhy reproduce the page declining, which is a
	// different outcome from a person having no picture.
	nullResult bool
	failWhy    string
	failStage  string

	tag       string
	timestamp int64
	stale     bool

	kicks int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// A LIBERACAO NAO E' KICK NEM LEITURA (H177): ela roda depois de a resposta
	// ser tomada, e caindo no ramo padrao ela vira lastScript e soma um kick.
	if strings.Contains(expr, "delete window.") {
		*out = "ok"
		return nil
	}
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		switch {
		case p.failWhy != "":
			stage := p.failStage
			if stage == "" {
				stage = "request"
			}
			*out = `{"stage":"` + stage + `","ok":false,"why":"` + p.failWhy + `"}`
		case p.nullResult:
			*out = `{"stage":"request","ok":false,"why":"NULL_RESULT"}`
		case p.present:
			*out = `{"stage":"done","ok":true,"why":"","present":true,` +
				`"url":"https://pps.whatsapp.net/v/full?token=abc",` +
				`"preview_url":"https://pps.whatsapp.net/v/preview?token=abc",` +
				`"tag":"` + p.tag + `","t":` + itoa(p.timestamp) + `,"stale":` + btoa(p.stale) + `}`
		default:
			// The measured no-picture shape: the page answered, the url fields
			// simply were not there.
			*out = `{"stage":"done","ok":true,"why":"","present":false,` +
				`"url":"","preview_url":"","tag":"` + p.tag + `","t":` +
				itoa(p.timestamp) + `,"stale":` + btoa(p.stale) + `}`
		}
		return nil
	}
	p.kicks++
	*out = `{"started":true}`
	return nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func btoa(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func fetcher(p *pageDouble) *Fetcher { return New(engine.NewRunner(), p.eval) }

func compressClock(t *testing.T) {
	t.Helper()
	ob, ot := requestBudget, requestTick
	requestBudget, requestTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { requestBudget, requestTick = ob, ot })
}

// TestNoPictureIsNotAnError is the contract this package exists to get right.
// Two of twelve sampled contacts had no picture, and reporting that as a
// failure would tell the caller "the fetch broke" where the truth is "there is
// nothing to fetch".
func TestNoPictureIsNotAnError(t *testing.T) {
	compressClock(t)
	got, err := fetcher(&pageDouble{present: false, tag: "t1", timestamp: 1787000000}).
		Fetch(context.Background(), "999@lid", "t/avatar")
	if err != nil {
		t.Fatalf("a contact without a picture produced an error: %v", err)
	}
	if got.Present {
		t.Fatalf("Present=true for a result carrying no url: %+v", got)
	}
	if got.URL != "" || got.PreviewURL != "" {
		t.Fatalf("absent avatar came back with urls: %+v", got)
	}
	// The metadata that DID arrive is still worth having.
	if got.Tag != "t1" || got.Timestamp.IsZero() {
		t.Fatalf("the fields the page did send were dropped: %+v", got)
	}
}

func TestPresentPictureCarriesBothURLs(t *testing.T) {
	compressClock(t)
	got, err := fetcher(&pageDouble{present: true, tag: "t2", timestamp: 1787000001, stale: true}).
		Fetch(context.Background(), "999@lid", "t/avatar")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !got.Present || got.URL == "" || got.PreviewURL == "" {
		t.Fatalf("a present avatar lost a url: %+v", got)
	}
	if got.Tag != "t2" || !got.Stale {
		t.Fatalf("tag or staleness dropped: %+v", got)
	}
	if got.Timestamp.Unix() != 1787000001 {
		t.Fatalf("timestamp=%v, want the server's", got.Timestamp)
	}
}

// TestThePageDecliningIsDistinctFromNoPicture. Both produce "no url", and
// collapsing them would make a broken page indistinguishable from an empty
// profile — the caller would retry neither, or both, and be wrong either way.
func TestThePageDecliningIsDistinctFromNoPicture(t *testing.T) {
	compressClock(t)
	_, err := fetcher(&pageDouble{nullResult: true}).Fetch(context.Background(), "999@lid", "t/avatar")
	if !errors.Is(err, ErrRequest) {
		t.Fatalf("got %v, want ErrRequest — a null answer is the page declining, "+
			"not a person without a picture", err)
	}
}

func TestUnusableIdentityIsErrBadJID(t *testing.T) {
	compressClock(t)
	if _, err := fetcher(&pageDouble{}).Fetch(context.Background(), "  ", "t/avatar"); !errors.Is(err, ErrBadJID) {
		t.Fatalf("got %v, want ErrBadJID for a blank jid", err)
	}
	p := &pageDouble{failStage: "resolve", failWhy: "WID_NULL"}
	if _, err := fetcher(p).Fetch(context.Background(), "nonsense", "t/avatar"); !errors.Is(err, ErrBadJID) {
		t.Fatalf("got %v, want ErrBadJID when the page could not build a wid", err)
	}
}

// TestABlankJIDNeverReachesThePage is ORDER: refusing must happen before any
// request goes out, or a caller's mistake becomes a call to WhatsApp.
func TestABlankJIDNeverReachesThePage(t *testing.T) {
	compressClock(t)
	p := &pageDouble{}
	if _, err := fetcher(p).Fetch(context.Background(), "", "t/avatar"); err == nil {
		t.Fatal("a blank jid was accepted")
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s) for a jid that was never valid", p.kicks)
	}
}

func TestThrowFromThePageIsErrRequest(t *testing.T) {
	compressClock(t)
	p := &pageDouble{failWhy: "boom"}
	_, err := fetcher(p).Fetch(context.Background(), "999@lid", "t/avatar")
	if !errors.Is(err, ErrRequest) {
		t.Fatalf("got %v, want ErrRequest", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("the page's reason was dropped: %v", err)
	}
}

// TestRedactionNeverPrintsTheURL. A profile-picture url identifies a person and
// carries an access token, so it is two secrets at once.
func TestRedactionNeverPrintsTheURL(t *testing.T) {
	a := Avatar{
		Present:    true,
		URL:        "https://pps.whatsapp.net/v/full?token=SECRET",
		PreviewURL: "https://pps.whatsapp.net/v/preview?token=SECRET",
		Tag:        "abc",
	}
	s := a.String()
	for _, secret := range []string{"SECRET", "pps.whatsapp.net", "/v/full"} {
		if strings.Contains(s, secret) {
			t.Fatalf("String() leaked %q: %s", secret, s)
		}
	}
	if absent := (Avatar{}).String(); !strings.Contains(absent, "present=false") {
		t.Fatalf("an absent avatar must say so: %s", absent)
	}
}

func TestCancelledContextNeverAsksTheServer(t *testing.T) {
	compressClock(t)
	p := &pageDouble{present: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fetcher(p).Fetch(ctx, "999@lid", "t/avatar"); err == nil {
		t.Fatal("a cancelled context produced an avatar")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the server %d time(s) for a caller that had given up", p.kicks)
	}
}
