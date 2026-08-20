package fetchmessages

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

type pageDouble struct {
	answer   string
	err      error
	lastExpr string
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	//
	// engine.Tab.Evaluate derives from the caller's context, so a cancelled or
	// expired ctx fails there. A double that ignored ctx would be better
	// behaved than the world in exactly the axis ARMADILHAS §1 warns about —
	// and it would let a capability fabricate a successful answer for a caller
	// that had already given up.
	if err := ctx.Err(); err != nil {
		return err
	}
	p.lastExpr = expr
	if p.err != nil {
		return p.err
	}
	*out = p.answer
	return nil
}

func fetcher(p *pageDouble) *Fetcher { return New(engine.NewRunner(), p.eval) }

const twoMessages = `{"ok":true,"loaded":384,"matched":2,"events":[
 {"jid":"a@c.us","id":{"id":"M1","from_me":false,"remote_jid":"a@c.us"},
  "direction":"in","type":"chat","t":1755600000},
 {"jid":"a@c.us","id":{"id":"M2","from_me":true,"remote_jid":"a@c.us"},
  "direction":"out","type":"image","t":1755600060}]}`

func TestFetchDecodesThroughMessagemeta(t *testing.T) {
	p := &pageDouble{answer: twoMessages}
	got, err := fetcher(p).Fetch(context.Background(), "a@c.us", 50, "t/f")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(got.Messages))
	}
	if got.Messages[0].ID.ID != "M1" || got.Messages[1].ID.ID != "M2" {
		t.Fatalf("order or ids wrong: %v", got.Messages)
	}
	if got.Messages[1].Type != "image" || got.Messages[1].ID.FromMe != true {
		t.Fatalf("second message decoded wrong: %+v", got.Messages[1])
	}
	if got.Loaded != 384 || got.Matched != 2 {
		t.Fatalf("loaded=%d matched=%d", got.Loaded, got.Matched)
	}
	if got.Truncated() {
		t.Fatal("Truncated()=true when matched == returned")
	}
}

// TestTheAllowListIsNotDuplicated is the point of importing MetaExpr instead of
// writing a second one. Two allow-lists drift; one of them then ships a body
// while the other's test still passes.
func TestTheAllowListIsNotDuplicated(t *testing.T) {
	p := &pageDouble{answer: `{"ok":true,"loaded":0,"matched":0,"events":[]}`}
	if _, err := fetcher(p).Fetch(context.Background(), "a@c.us", 10, "t/f"); err != nil {
		t.Fatal(err)
	}
	expr := p.lastExpr
	for _, forbidden := range []string{"body", "caption", "media", "text"} {
		if strings.Contains(strings.ToLower(expr), forbidden) {
			t.Fatalf("the fetch script mentions %q; WaMessageMeta is metadata-only", forbidden)
		}
	}
	// The shared expression must literally be the one messagemeta owns.
	if !strings.Contains(expr, "from_me: !!key.fromMe") {
		t.Fatal("the fetch script does not carry messagemeta's allow-list; if it grew its " +
			"own, the two can drift and only one of them is covered by the body test")
	}
}

// TestChatJIDIsQuotedIntoTheScript guards a page-side injection. The jid comes
// from a caller and lands inside a JavaScript expression that runs with this
// session's privileges.
func TestChatJIDIsQuotedIntoTheScript(t *testing.T) {
	p := &pageDouble{answer: `{"ok":true,"loaded":0,"matched":0,"events":[]}`}
	hostile := `x"; window.__pwned = 1; const y = "`
	if _, err := fetcher(p).Fetch(context.Background(), hostile, 10, "t/f"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.lastExpr, "window.__pwned") && !strings.Contains(p.lastExpr, `\"`) {
		t.Fatalf("the chat jid was concatenated raw into the script:\n%s", p.lastExpr)
	}
	if !strings.Contains(p.lastExpr, `\"`) {
		t.Fatal("the chat jid was not escaped; a quote in a jid would end the string " +
			"literal and everything after it would execute in the page")
	}
}

// TestEmptyChatMeansNoFilterNotAWildcard pins a semantic that is easy to get
// backwards later: an empty jid reads every loaded chat because there is no
// filter, not because "" matches everything.
func TestEmptyChatMeansNoFilterNotAWildcard(t *testing.T) {
	p := &pageDouble{answer: `{"ok":true,"loaded":5,"matched":5,"events":[]}`}
	if _, err := fetcher(p).Fetch(context.Background(), "", 10, "t/f"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.lastExpr, `want !== ""`) {
		t.Fatal("the script does not treat an empty jid as 'no filter'")
	}
}

// TestTruncationIsVisible: a caller that asked for 10 of 400 must be able to
// tell it got a window, not a history. Silence here is the incompleteness this
// module keeps paying for.
func TestTruncationIsVisible(t *testing.T) {
	p := &pageDouble{answer: `{"ok":true,"loaded":384,"matched":97,"events":[
		{"jid":"a@c.us","id":{"id":"M1","from_me":false,"remote_jid":"a@c.us"},
		 "direction":"in","type":"chat","t":1}]}`}
	got, err := fetcher(p).Fetch(context.Background(), "a@c.us", 1, "t/f")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Truncated() {
		t.Fatalf("matched=%d returned=%d and Truncated()=false; the caller would read a "+
			"window as a whole history", got.Matched, len(got.Messages))
	}
	if got.Loaded != 384 {
		t.Fatalf("Loaded=%d: without it a short result cannot be told from an empty page",
			got.Loaded)
	}
}

func TestMissingCollectionIsItsOwnError(t *testing.T) {
	p := &pageDouble{answer: `{"ok":false}`}
	_, err := fetcher(p).Fetch(context.Background(), "a@c.us", 10, "t/f")
	if !errors.Is(err, ErrNoCollection) {
		t.Fatalf("err=%v, want ErrNoCollection", err)
	}
}

func TestProbeFailureIsNotAnEmptyResult(t *testing.T) {
	boom := errors.New("evaluate exploded")
	_, err := fetcher(&pageDouble{err: boom}).Fetch(context.Background(), "a@c.us", 10, "t/f")
	if err == nil {
		t.Fatal("a failed probe produced no error; an empty result would read as 'no messages'")
	}
	if errors.Is(err, ErrNoCollection) {
		t.Fatal("a probe failure was reported as a missing collection")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v, want it to wrap the probe error", err)
	}
}

func TestRenderingStillRedacts(t *testing.T) {
	p := &pageDouble{answer: twoMessages}
	got, err := fetcher(p).Fetch(context.Background(), "a@c.us", 50, "t/f")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Messages[0].String(), "a@c.us") {
		t.Fatalf("a fetched Meta leaked the jid when rendered: %s", got.Messages[0])
	}
}

// TestCancelledContextIsNotAFabricatedSuccess exercises the axis the doubles
// used to ignore.
//
// engine.Tab.Evaluate derives from the caller's context, so in production a
// cancelled ctx fails. The doubles in this file returned their canned answer
// regardless, which made every capability test blind to cancellation: a
// capability that swallowed the error from runner.Do would still pass.
//
// A caller that has given up must not receive a manufactured answer — that is
// worse than an error, because it looks like data.
func TestCancelledContextIsNotAFabricatedSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := func() error {
		_, e := fetcher(&pageDouble{answer: twoMessages}).Fetch(ctx, "a@c.us", 5, "t/cancel")
		return e
	}(); err == nil {
		t.Fatal("a cancelled context produced a successful answer; the caller had already " +
			"given up and got manufactured data instead of an error")
	}
}
