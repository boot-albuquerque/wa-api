package search

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type double struct {
	answer     string
	lastScript string
	kicks      int
}

func (d *double) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		*out = d.answer
		return nil
	}
	d.kicks++
	d.lastScript = expr
	*out = "kicked"
	return nil
}

func srch(d *double) *Searcher { return New(engine.NewRunner(), d.eval) }

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

const twoHits = `{"ok":true,"eof":false,"hits":[
 {"chat":"1@lid","id":"M1","fromMe":true,"type":"chat","t":1700000000},
 {"chat":"2@g.us","id":"M2","fromMe":false,"type":"image","t":0}]}`

func TestAHitIsAnAddress(t *testing.T) {
	d := &double{answer: twoHits}
	got, err := srch(d).Messages(context.Background(), "termo", 1, "t")
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if got.Returned != 2 || len(got.Hits) != 2 {
		t.Fatalf("returned %d hits", got.Returned)
	}
	if got.Hits[0].MessageID != "M1" || got.Hits[1].ChatJID != "2@g.us" {
		t.Errorf("the addresses did not survive: %+v", got.Hits)
	}
	if got.Hits[0].At.IsZero() {
		t.Error("a timestamp was dropped")
	}
	if !got.Hits[1].At.IsZero() {
		t.Error("an absent timestamp became a real instant")
	}
}

// NO BODY CAN REACH GO, and the guard is the SCRIPT: the projection there is
// what makes it impossible, not a filter on this side. A version that serialised
// the message and picked fields in Go would have had the body in the answer
// first — the same mistake H107 refused for rawData.
func TestNoBodyCanCrossFromThePage(t *testing.T) {
	d := &double{answer: twoHits}
	if _, err := srch(d).Messages(context.Background(), "termo", 1, "t"); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	code := withoutComments(d.lastScript)
	for _, forbidden := range []string{"body", "caption", "serialize()", "JSON.stringify(m)"} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the search script mentions %q", forbidden)
		}
	}
	// And the Go type has nowhere to put one.
	h := Hit{ChatJID: "1@lid", MessageID: "M1"}
	if s := h.String(); strings.Contains(s, "1@lid") || strings.Contains(s, "M1") {
		t.Errorf("Hit.String carries identity: %s", s)
	}
}

// NO MATCHES IS NOT AN ERROR. Measured: a four-letter term found twenty and the
// single letter "a" found zero, so an empty answer is a normal outcome and a
// caller must not be told the search failed.
func TestNoMatchesIsNotAnError(t *testing.T) {
	d := &double{answer: `{"ok":true,"eof":true,"hits":[]}`}
	got, err := srch(d).Messages(context.Background(), "zzzz", 1, "t")
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if got.Hits == nil {
		t.Error("an empty search returned nil rather than an empty slice")
	}
	if !got.EOF {
		t.Error("the page's own eof flag was dropped")
	}
}

// THERE IS NO CHAT SCOPE, and its absence is deliberate: passing the chat jid as
// the fourth argument — exactly what the reference does with options.chatId —
// measured 0 hits with eof, against 20 unscoped. An option that returns nothing
// is worse than an absent one, because the empty answer reads as "no matches".
func TestTheSearchIsNotScopedToAChat(t *testing.T) {
	d := &double{answer: twoHits}
	if _, err := srch(d).Messages(context.Background(), "termo", 1, "t"); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	code := withoutComments(d.lastScript)
	if !strings.Contains(code, "undefined)") {
		t.Fatal("the script does not pass undefined as the remote argument; a chat " +
			"scope measured zero hits and must not be reintroduced silently")
	}
}

func TestAnEmptyQueryNeverReachesThePage(t *testing.T) {
	d := &double{answer: twoHits}
	if _, err := srch(d).Messages(context.Background(), "   ", 1, "t"); !errors.Is(err, ErrNoQuery) {
		t.Fatalf("err = %v, want ErrNoQuery", err)
	}
	if d.kicks != 0 {
		t.Error("an empty query reached the page")
	}
}

// The query reaches the page as DATA. A quote in a search term would otherwise
// end the string and change what runs.
func TestTheQueryIsQuotedIntoTheScript(t *testing.T) {
	d := &double{answer: twoHits}
	if _, err := srch(d).Messages(context.Background(), `a"b`, 3, "t"); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if !strings.Contains(d.lastScript, `"a\"b"`) {
		t.Error("the query was not quoted into the script")
	}
	if !strings.Contains(d.lastScript, "\n\t\t\t\t3,") {
		t.Error("the page number did not reach the script")
	}
}

func TestAPageBelowOneIsNormalised(t *testing.T) {
	d := &double{answer: twoHits}
	if _, err := srch(d).Messages(context.Background(), "x", 0, "t"); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if !strings.Contains(d.lastScript, "\n\t\t\t\t1,") {
		t.Error("page 0 was not normalised to 1")
	}
}

func TestACancelledCallerNeverSearches(t *testing.T) {
	d := &double{answer: twoHits}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := srch(d).Messages(ctx, "x", 1, "t"); err == nil {
		t.Fatal("a cancelled context produced results")
	}
	if d.kicks != 0 {
		t.Error("the page was searched for a caller that had given up")
	}
}

func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	d := &double{answer: ""}
	_, err := srch(d).Messages(context.Background(), "x", 1, "t")
	if err == nil || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}
