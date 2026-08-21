package call

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type double struct {
	answer       string
	pendingReads int

	reads      int
	kicks      int
	lastScript string
}

func (d *double) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		d.reads++
		if d.reads <= d.pendingReads {
			*out = ""
			return nil
		}
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func mgr(d *double) *Manager { return New(engine.NewRunner(), d.eval) }

func soon() time.Time { return time.Now().Add(time.Hour) }

// THE KIND IS ENUMERATED AND CHECKED BEFORE THE PAGE IS TOUCHED. The upstream
// throws on anything outside {voice, video}; a typo that reached the server
// would come back as something unrecognisable, if it came back at all.
func TestOnlyVoiceAndVideoAreAccepted(t *testing.T) {
	for _, k := range []Kind{"", "audio", "Video", "VOICE", "vídeo"} {
		d := &double{answer: `{"ok":true,"link":"x"}`}
		if _, err := mgr(d).CreateLink(context.Background(), soon(), k, "t"); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("CreateLink(%q) = %v, want ErrUnknownKind", k, err)
		}
		if d.kicks != 0 {
			t.Errorf("%q reached the page", k)
		}
	}
}

// A START TIME IN THE PAST IS REFUSED HERE. The page takes it and produces a
// link to a moment that has gone by — a link nobody can use and no error
// anybody sees.
func TestAStartTimeInThePastIsRefused(t *testing.T) {
	for _, at := range []time.Time{{}, time.Now().Add(-time.Minute), time.Unix(0, 0)} {
		d := &double{answer: `{"ok":true,"link":"x"}`}
		if _, err := mgr(d).CreateLink(context.Background(), at, KindVideo, "t"); !errors.Is(err, ErrPastStart) {
			t.Errorf("CreateLink(%v) = %v, want ErrPastStart", at, err)
		}
		if d.kicks != 0 {
			t.Error("a past start time reached the page")
		}
	}
}

// THE PAGE IS HANDED UNIX SECONDS AND NEVER ASKED WHAT TIME IT IS.
//
// The upstream does Math.floor(startTime.getTime()/1000) inside the evaluate;
// here that arithmetic is on this side, because a page that reads its own clock
// is the invariant this module has a gate for.
func TestTheTimestampIsComputedInGo(t *testing.T) {
	at := time.Now().Add(90 * time.Minute).Truncate(time.Second)
	d := &double{answer: `{"ok":true,"link":"https://call.whatsapp.com/video/xxxx"}`}
	got, err := mgr(d).CreateLink(context.Background(), at, KindVideo, "t")
	if err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if !strings.Contains(d.lastScript, "createEventCallLink("+itoa(at.Unix())+", \"video\")") {
		t.Fatalf("the page did not receive the unix seconds and the kind; script had:\n%s", d.lastScript)
	}
	for _, banned := range []string{"Date.now(", "new Date(", "setTimeout("} {
		if strings.Contains(d.lastScript, banned) {
			t.Errorf("the link script contains %q", banned)
		}
	}
	if got.StartsAt != at || got.Kind != KindVideo {
		t.Errorf("got %+v", got)
	}
}

// AN EMPTY LINK IS NOT A LINK.
//
// The upstream returns `response ?? ''` and calls that success. A caller handed
// an empty string would publish it, or would join nothing and blame the call.
func TestAnEmptyLinkIsAnError(t *testing.T) {
	for _, ans := range []string{`{"ok":true,"link":""}`, `{"ok":true,"link":"   "}`, `{"ok":true}`} {
		d := &double{answer: ans}
		if _, err := mgr(d).CreateLink(context.Background(), soon(), KindVoice, "t"); !errors.Is(err, ErrNoLink) {
			t.Errorf("answer %s gave %v, want ErrNoLink", ans, err)
		}
	}
}

// THE LINK IS A CREDENTIAL. Anybody holding it can join the call, so it is
// treated exactly like a group invite code: never rendered.
func TestTheLinkIsNeverRendered(t *testing.T) {
	l := Link{URL: "https://call.whatsapp.com/video/SECRETCODE", Kind: KindVideo, StartsAt: soon()}
	s := l.String()
	if strings.Contains(s, "SECRETCODE") || strings.Contains(s, "call.whatsapp.com") {
		t.Fatalf("Link.String carries the link: %s", s)
	}
	if !strings.Contains(s, "len=") {
		t.Errorf("the rendering says nothing useful: %s", s)
	}
}

// A rejection needs both halves, and neither is guessable.
func TestARejectionNeedsACallerAndACallID(t *testing.T) {
	for _, c := range [][2]string{{"", "id"}, {"jid", ""}, {"  ", "  "}} {
		d := &double{answer: `{"ok":true}`}
		if err := mgr(d).Reject(context.Background(), c[0], c[1], "t"); !errors.Is(err, ErrNoCall) {
			t.Errorf("Reject(%q,%q) = %v, want ErrNoCall", c[0], c[1], err)
		}
		if d.kicks != 0 {
			t.Error("an incomplete rejection reached the page")
		}
	}
}

// THE REJECT STANZA CARRIES THE CALLER AS BOTH `to` AND `call-creator`.
//
// Those are two different fields with the same value, and dropping either makes
// a stanza the server ignores silently — which looks exactly like a rejection
// that worked.
func TestTheRejectStanzaIsShapedLikeTheProtocolWantsIt(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if err := mgr(d).Reject(context.Background(), "5541999999999@c.us", "CALLID123", "t"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	for _, want := range []string{
		`"call-id": "CALLID123"`,
		`"call-creator": "5541999999999@c.us"`,
		`to: "5541999999999@c.us"`,
		`count: "0"`,
	} {
		if !strings.Contains(d.lastScript, want) {
			t.Errorf("the stanza is missing %s", want)
		}
	}
	// The `from` is the account's own user, asked of the page rather than
	// passed in: a caller does not know its own jid and should not have to.
	if !strings.Contains(d.lastScript, "getMaybeMePnUser()") {
		t.Error("the stanza does not take `from` from the page")
	}
}

// A page failure keeps its reason, on both methods.
func TestPageFailuresKeepTheirReason(t *testing.T) {
	d := &double{answer: `{"ok":false,"why":"TypeError message=nope keys=[a]"}`}
	if _, err := mgr(d).CreateLink(context.Background(), soon(), KindVideo, "t"); !errors.Is(err, ErrLink) ||
		!strings.Contains(err.Error(), "nope") {
		t.Errorf("link err = %v", err)
	}
	d2 := &double{answer: `{"ok":false,"why":"NO_ME_USER"}`}
	if err := mgr(d2).Reject(context.Background(), "a@c.us", "id", "t"); !errors.Is(err, ErrReject) ||
		!strings.Contains(err.Error(), "NO_ME_USER") {
		t.Errorf("reject err = %v", err)
	}
}

// The parked loop is bounded and polls.
func TestTheParkedLoopIsBounded(t *testing.T) {
	oldB, oldT := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = oldB, oldT }()

	d := &double{pendingReads: 1 << 30}
	if _, err := mgr(d).CreateLink(context.Background(), soon(), KindVideo, "t"); err == nil ||
		!strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}

	d2 := &double{pendingReads: 2, answer: `{"ok":true,"link":"https://x/y"}`}
	Budget = 5 * time.Second
	if _, err := mgr(d2).CreateLink(context.Background(), soon(), KindVideo, "t"); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	if d2.reads < 3 {
		t.Errorf("%d reads; the loop did not poll", d2.reads)
	}
}

// Pending counts and says nothing else. A call model carries the caller's
// identity, and this is the reader a diagnostic would reach for first.
func TestPendingCountsAndNothingElse(t *testing.T) {
	d := &double{answer: `{"ok":true,"count":2}`}
	n, err := mgr(d).Pending(context.Background(), "t")
	if err != nil || n != 2 {
		t.Fatalf("Pending = %d, %v", n, err)
	}
	if strings.Contains(d.lastScript, "peerJid") || strings.Contains(d.lastScript, "_serialized") {
		t.Error("the pending script reads a caller identity")
	}
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
