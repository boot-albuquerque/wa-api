package status

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

// AN EMPTY FEED LIST IS A VALID ANSWER, not an error. Statuses arrive with the
// account's own sync; a session holding none is an ordinary state, and turning
// it into a failure would make every quiet account look broken.
func TestAnEmptyListIsNotAnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"feeds":[]}`}
	got, err := rd(d).List(context.Background(), "t")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d feeds, want none", len(got))
	}
}

// A page failure IS an error, and keeps its reason. Zero feeds and a broken read
// must not look the same — the whole point of the distinction poll and
// groupreq both draw.
func TestAPageFailureIsNotAnEmptyList(t *testing.T) {
	d := &double{answer: `{"ok":false,"why":"NO_STATUS_COLLECTION"}`}
	_, err := rd(d).List(context.Background(), "t")
	if !errors.Is(err, ErrRead) || !strings.Contains(err.Error(), "NO_STATUS_COLLECTION") {
		t.Fatalf("err = %v", err)
	}
}

// The counts and the timestamp survive the boundary, and a zero timestamp stays
// zero rather than becoming the epoch.
func TestTheCountsAndTimeSurvive(t *testing.T) {
	d := &double{answer: `{"ok":true,"feeds":[` +
		`{"id":"1@c.us","total":4,"unread":1,"read":3,"t":1787000000,"loading":false},` +
		`{"id":"2@c.us","total":0,"unread":0,"read":0,"t":0,"loading":true}]}`}
	got, err := rd(d).List(context.Background(), "t")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d feeds", len(got))
	}
	if got[0].Total != 4 || got[0].Unread != 1 || got[0].Read != 3 {
		t.Errorf("counts lost: %+v", got[0])
	}
	if got[0].At.IsZero() {
		t.Error("a feed with a timestamp came back with a zero time")
	}
	if !got[1].At.IsZero() {
		t.Error("a feed with t=0 was given a time; the epoch is not a timestamp")
	}
	if !got[1].Loading {
		t.Error("the loading flag was dropped")
	}
}

// "NO FEED" AND "NO SUCH CONTACT" ARE THE SAME PAGE ANSWER AND A DIFFERENT
// MEANING. The collection holds what this session knows, so an absent feed is
// its own error rather than a Feed full of zeros — which a caller would render
// as "this person posted nothing".
func TestAnAbsentFeedIsItsOwnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"found":false}`}
	_, err := rd(d).ByContact(context.Background(), "1@c.us", "t")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// An empty contact never reaches the page.
func TestAnEmptyContactIsRefused(t *testing.T) {
	d := &double{answer: `{"ok":true}`}
	if _, err := rd(d).ByContact(context.Background(), "   ", "t"); !errors.Is(err, ErrNoContact) {
		t.Errorf("err = %v, want ErrNoContact", err)
	}
	if d.kicks != 0 {
		t.Error("an empty contact reached the page")
	}
}

// BOTH IDENTITY FORMS ARE TRIED. This build files under LID and a caller holds
// whichever form it was handed; answering "not found" for the wrong key would be
// a claim about the account rather than about the argument.
func TestTheLookupTriesBothIdentityForms(t *testing.T) {
	d := &double{answer: `{"ok":true,"found":false}`}
	_, _ = rd(d).ByContact(context.Background(), "1@c.us", "t")
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, `S.get("1@c.us")`) {
		t.Error("the lookup does not try the jid as given")
	}
	if !strings.Contains(code, "createWid(") || !strings.Contains(code, "S.get(wid)") {
		t.Error("the lookup does not try the wid form")
	}
}

// THE PAGE'S OWN GETTERS ARE USED, not the model's fields.
//
// H51 measured what reaching past a getter costs: the obvious field answered for
// 1 record in 384, and a doubled test suite would never have noticed. This is
// the one place a test can hold that line, because a double returns whatever it
// is told.
func TestTheReadGoesThroughTheGetters(t *testing.T) {
	d := &double{answer: `{"ok":true,"feeds":[]}`}
	_, _ = rd(d).List(context.Background(), "t")
	code := withoutComments(d.lastScript)
	for _, want := range []string{"G.getTotalCount", "G.getUnreadCount", "G.getReadCount", "G.getT"} {
		if !strings.Contains(code, want) {
			t.Errorf("the read does not use %s", want)
		}
	}
	// And it must not read the raw storage, which is what the getters exist to
	// hide.
	if strings.Contains(code, "__x_") {
		t.Error("the read reaches into the model's raw storage")
	}
}

// NOTHING IN THIS PACKAGE SENDS. The build exports sendStatusTextMsgAction and
// sendStatusMediaMsgAction; a status is visible to every contact in the address
// book, measured at 944 on this account. That is a blast radius, not a scope
// decision, and this test is where the decision is kept.
func TestThisPackageDoesNotPost(t *testing.T) {
	d := &double{answer: `{"ok":true,"feeds":[]}`}
	_, _ = rd(d).List(context.Background(), "t")
	_, _ = rd(d).ByContact(context.Background(), "1@c.us", "t")
	for _, banned := range []string{"sendStatusTextMsgAction", "sendStatusMediaMsgAction",
		"WAWebSendStatusMsgAction"} {
		if strings.Contains(withoutComments(d.lastScript), banned) {
			t.Errorf("a script in this package references %s", banned)
		}
	}
}

// The rendering carries no identity.
func TestTheFeedIsQuiet(t *testing.T) {
	f := Feed{ContactJID: "5541999999999@c.us", Total: 3}
	if strings.Contains(f.String(), "5541") {
		t.Errorf("Feed.String carries the contact: %s", f.String())
	}
}

// The parked loop is bounded.
func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	d := &double{pendingReads: 1 << 30}
	if _, err := rd(d).List(context.Background(), "t"); err == nil ||
		!strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}
