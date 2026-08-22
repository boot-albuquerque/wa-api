package channel

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

func rd(d *double) *Reader { return New(engine.NewRunner(), d.eval) }

func withoutComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// A CALLER HOLDS A LINK, NOT A CODE. Making them split it is making them know
// something this package already knows, and a link pasted whole is the shape
// that actually reaches a program.
func TestTheWholeLinkIsAccepted(t *testing.T) {
	for _, in := range []string{
		"0029Vabc",
		"https://whatsapp.com/channel/0029Vabc",
		"https://whatsapp.com/channel/0029Vabc?x=1",
		"  https://whatsapp.com/channel/0029Vabc#frag  ",
	} {
		if got := normalizeCode(in); got != "0029Vabc" {
			t.Errorf("normalizeCode(%q) = %q", in, got)
		}
	}
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).ByInviteCode(context.Background(), "   ", "t"); !errors.Is(err, ErrNoCode) {
		t.Errorf("err = %v, want ErrNoCode", err)
	}
	if d.kicks != 0 {
		t.Error("an empty code reached the page")
	}
}

// THE VALUES LIVE IN MIXINS, and reading the obvious field returns undefined
// forever.
//
// There is no name at the top level; there is
// newsletterNameMetadataMixin.nameElementValue. This is the fourth time this
// repository has met that class of defect, and the only place a test can hold
// the line — a double returns whatever it is told.
func TestTheReadGoesThroughTheMixins(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	_, _ = rd(d).ByInviteCode(context.Background(), "0029Vabc", "t")
	code := withoutComments(d.lastScript)
	for _, want := range []string{
		"newsletterNameMetadataMixin",
		"nameElementValue",
		"newsletterSubscribersMetadataMixin",
		"subscribersCount",
		"descriptionQueryDescriptionResponseMixin",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("the script does not read %s", want)
		}
	}
	// And it must not read the flat fields the reference implies.
	if strings.Contains(code, "r.name") || strings.Contains(code, "r.subscribersCount") {
		t.Error("the script reads a flat field; those are undefined on this build")
	}
}

// FOLLOWING IS DERIVED FROM THE MEMBERSHIP MIXIN EXISTING.
//
// It comes back null for a channel this account does not follow — and that is
// exactly what let this reader be proven without following anything. A hardcoded
// false would have made the field a lie the day somebody followed one.
func TestFollowingComesFromTheMembershipMixin(t *testing.T) {
	d := &double{answer: `{"ok":true,"member":false,"jid":"1@newsletter"}`}
	got, err := rd(d).ByInviteCode(context.Background(), "0029Vabc", "t")
	if err != nil {
		t.Fatalf("ByInviteCode: %v", err)
	}
	if got.Following {
		t.Error("a channel with a null membership mixin reads as followed")
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, `mix("newsletterMembershipMetadataMixin")`) {
		t.Error("membership is not derived from the mixin")
	}
	if strings.Contains(code, "member: false") {
		t.Error("membership is hardcoded")
	}
}

// An unknown code is its own answer, not a read failure: the repairs differ.
func TestAnUnknownCodeIsItsOwnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"notFound":true}`}
	if _, err := rd(d).ByInviteCode(context.Background(), "0029Vabc", "t"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	d2 := &double{answer: `{"ok":false,"why":"TypeError message=nope"}`}
	_, err := rd(d2).ByInviteCode(context.Background(), "0029Vabc", "t")
	if !errors.Is(err, ErrRead) || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrRead and not ErrNotFound", err)
	}
}

// The fields survive the boundary, and a zero timestamp stays zero.
func TestTheFieldsSurvive(t *testing.T) {
	d := &double{answer: `{"ok":true,"jid":"12@newsletter","code":"0029Vabc","name":"Canal",` +
		`"description":"d","subscribers":4200,"state":"ACTIVE","verification":"VERIFIED",` +
		`"createdAt":1700000000,"member":true,"picture":true}`}
	got, err := rd(d).ByInviteCode(context.Background(), "0029Vabc", "t")
	if err != nil {
		t.Fatalf("ByInviteCode: %v", err)
	}
	if got.Subscribers != 4200 || got.State != "ACTIVE" || got.Verification != "VERIFIED" {
		t.Errorf("fields lost: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("a channel with a creation time came back with a zero time")
	}
	if !got.Following || !got.HasPicture {
		t.Errorf("booleans lost: %+v", got)
	}

	d2 := &double{answer: `{"ok":true,"jid":"12@newsletter","createdAt":0}`}
	got2, _ := rd(d2).ByInviteCode(context.Background(), "0029Vabc", "t")
	if !got2.CreatedAt.IsZero() {
		t.Error("createdAt 0 was turned into the epoch")
	}
}

// THE PICTURE URL DOES NOT CROSS. It is large, it expires, and nothing here
// needs it — only whether there is one.
func TestThePictureUrlDoesNotCross(t *testing.T) {
	d := &double{answer: `{"ok":true,"picture":true}`}
	_, _ = rd(d).ByInviteCode(context.Background(), "0029Vabc", "t")
	code := withoutComments(d.lastScript)
	if strings.Contains(code, "picture: str(") || strings.Contains(code, "pictureUrl") {
		t.Error("the script carries a picture url")
	}
	if !strings.Contains(code, "picture: !!(") {
		t.Error("the picture is not reduced to a boolean")
	}
}

// The rendering carries no content.
func TestTheRenderingIsQuiet(t *testing.T) {
	c := Channel{JID: "12345@newsletter", Name: "Canal do Fulano", Description: "segredo",
		Subscribers: 10, State: "ACTIVE"}
	s := c.String()
	if strings.Contains(s, "Fulano") || strings.Contains(s, "segredo") || strings.Contains(s, "12345") {
		t.Errorf("Channel.String carries content: %s", s)
	}
}

// The parked loop is bounded.
func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	d := &double{pendingReads: 1 << 30}
	if _, err := rd(d).ByInviteCode(context.Background(), "0029Vabc", "t"); err == nil ||
		!strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}
