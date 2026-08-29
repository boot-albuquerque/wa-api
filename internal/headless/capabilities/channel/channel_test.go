package channel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
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
	if strings.Contains(expr, "delete window."+stateKeyPrefix) || strings.HasPrefix(expr, "window."+stateKeyPrefix) {
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

// A DIRECTORY RESULT IS A MODEL, NOT THE MIXIN BAG, and this double imitates
// that REAL rule rather than the convenient one. Measured 2026-08-22 over 50
// live results: the values live in __x_-prefixed fields of
// __x_newsletterMetadata (__x_size number 50/50, __x_verified boolean 50/50,
// __x_membershipType string 50/50), and __x_state is UNDEFINED on all 50.
//
// The first version of this reader looked for the mixin names — the vocabulary
// the metadata query uses — and came back with 50 results carrying a name and a
// jid and NOTHING else. That failure looks like a thin directory rather than
// like a bug, which is why the shape is measured and not assumed.
const oneDirectoryResult = `{"ok":true,"returned":2,"results":[
 {"jid":"111@newsletter","name":"n1","description":"d1","subscribers":5114818,
  "verified":true,"membership":"guest","createdAt":1700000000},
 {"jid":"222@newsletter","name":"n2","description":"","subscribers":0,
  "verified":false,"membership":"guest","createdAt":0}]}`

func TestSearchReadsEveryResult(t *testing.T) {
	d := &double{answer: oneDirectoryResult}
	got, err := rd(d).Search(context.Background(), SearchOptions{}, "t")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Subscribers != 5114818 {
		t.Errorf("subscribers = %d; seven digits must survive", got[0].Subscribers)
	}
	// ZERO SUBSCRIBERS IS A LEGITIMATE ANSWER, not an absent one.
	if got[1].JID == "" {
		t.Error("a channel with zero subscribers lost its identity")
	}
	if !got[0].Verified || got[1].Verified {
		t.Error("verification was not carried per result")
	}
	if got[0].CreatedAt.IsZero() {
		t.Error("a creation time was dropped")
	}
	if !got[1].CreatedAt.IsZero() {
		t.Error("an absent creation time became a real instant")
	}
	if s := got[0].String(); strings.Contains(s, "n1") || strings.Contains(s, "d1") {
		t.Errorf("DirectoryEntry.String carries content: %s", s)
	}
}

// THE SCRIPT MUST READ THE FIELDS THAT EXIST, which the check above cannot see:
// the double supplies the parsed results. Sixth time this session that a rule
// living in the script needed its own assertion.
func TestTheSearchScriptReadsTheMeasuredFields(t *testing.T) {
	d := &double{answer: oneDirectoryResult}
	if _, err := rd(d).Search(context.Background(), SearchOptions{}, "t"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, fieldNewsletterMetadata) {
		t.Fatal("the search script does not read " + fieldNewsletterMetadata)
	}
	for _, f := range []string{fieldSize, fieldVerified, fieldMembership, fieldName} {
		if !strings.Contains(code, f) {
			t.Errorf("the search script does not read %s", f)
		}
	}
	// AND IT MUST NOT READ THE MIXIN NAMES HERE. Those belong to the metadata
	// query; reading them on a directory result returns empty for every channel.
	for _, m := range []string{mixSubscribers, mixVerify, mixName} {
		if strings.Contains(code, m) {
			t.Errorf("the search script reads %s, which is the metadata query's "+
				"vocabulary and is absent from a directory result", m)
		}
	}
}

// THE PAGE IS NOT PATCHED. The reference changes the directory page size by
// overwriting a page function and putting it back afterwards. That function does
// not exist on this build (measured: pageSizeFn=false), and patching a global
// means a failed restore leaves the page altered for every later caller.
func TestTheSearchScriptDoesNotPatchThePage(t *testing.T) {
	d := &double{answer: oneDirectoryResult}
	if _, err := rd(d).Search(context.Background(), SearchOptions{}, "t"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	code := withoutComments(d.lastScript)
	for _, patch := range []string{"getNewsletterDirectoryPageSize", "= () =>", "= function"} {
		if strings.Contains(code, patch) {
			t.Errorf("the search script contains %q, which alters the page", patch)
		}
	}
}

// AN EMPTY DIRECTORY IS NOT AN ERROR. A search for a term nobody used returns
// nothing, and turning that into a failure makes a caller retry forever.
func TestAnEmptyDirectoryIsNotAnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"returned":0,"results":[]}`}
	got, err := rd(d).Search(context.Background(), SearchOptions{Query: "zzz"}, "t")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got == nil {
		t.Error("an empty search returned nil rather than an empty list; a caller " +
			"ranging over it should not have to nil-check")
	}
	if len(got) != 0 {
		t.Errorf("got %d results for an empty answer", len(got))
	}
}

func TestASearchRefusalIsErrRead(t *testing.T) {
	d := &double{answer: `{"ok":false,"why":"boom"}`}
	if _, err := rd(d).Search(context.Background(), SearchOptions{}, "t"); !errors.Is(err, ErrRead) {
		t.Fatalf("err = %v, want ErrRead", err)
	}
}

// The query and the region reach the page as data, never concatenated raw.
func TestTheQueryReachesThePageQuoted(t *testing.T) {
	d := &double{answer: oneDirectoryResult}
	if _, err := rd(d).Search(context.Background(),
		SearchOptions{Query: `he"llo`, Region: "BR", SkipSubscribed: true}, "t"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	code := d.lastScript
	if !strings.Contains(code, `"he\"llo"`) {
		t.Error("the query was not quoted into the script; a quote in a search term " +
			"would end the string and change what runs")
	}
	if !strings.Contains(code, `"BR"`) {
		t.Error("the region did not reach the page")
	}
	if !strings.Contains(code, "skipSubscribedNewsletters: true") {
		t.Error("SkipSubscribed did not reach the page")
	}
}
