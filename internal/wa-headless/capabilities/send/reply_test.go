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

// replyDouble answers the three scripts a reply uses: the kick, the parked
// result, the message-collection verify, and the quote check.
type replyDouble struct {
	resolvedJID string
	failStage   string
	failWhy     string

	storedUnder string
	storedAt    time.Time
	// carriesQuote is what the quote check reports for the sent message. It is
	// SEPARATE from the send succeeding, because that is exactly the failure
	// this capability exists to catch: the text arrives either way.
	carriesQuote bool

	kicks      int
	lastScript string
}

func (p *replyDouble) eval(ctx context.Context, expr string, out *string) error {
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

	// ROUTING ORDER MATTERS HERE, and getting it wrong cost two runs. THREE of
	// the four scripts walk the message collection, so "getModelsArray" cannot
	// tell them apart: the kick was being routed to the verify branch, which
	// meant lastScript was never recorded and an assertion about the kick
	// failed for a reason that had nothing to do with the code under test.
	//
	// Each branch now keys on something only that script contains.
	switch {
	case strings.Contains(expr, replyStateKey+`"] = {`):
		p.kicks++
		p.lastScript = expr
		*out = `{"started":true}`
		return nil
	case strings.Contains(expr, "quotedStanzaID"):
		*out = fmt.Sprintf(`{"found":true,"quoted":%t}`, p.carriesQuote)
		return nil
	case strings.Contains(expr, "getModelsArray"):
		if p.storedAt.IsZero() {
			*out = `[]`
			return nil
		}
		*out = fmt.Sprintf(
			`[{"jid":%q,"id":{"id":"REPLY1","from_me":true,"remote_jid":%q},`+
				`"direction":"out","type":"chat","t":%d}]`,
			p.storedUnder, p.storedUnder, p.storedAt.Unix())
		return nil
	case strings.Contains(expr, "const s = window["):
		if p.failWhy != "" {
			stage := p.failStage
			if stage == "" {
				stage = "dispatch"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.failWhy)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","jid":%q}`, p.resolvedJID)
		return nil
	default:
		*out = `{"started":true}`
		return nil
	}
}

func replyer(p *replyDouble) func(string) (Result, error) {
	return func(quotedID string) (Result, error) {
		return Reply(context.Background(), engine.NewRunner(), p.eval,
			phoneJID, quotedID, "resposta", "t/reply")
	}
}

// TestAReplyWithoutAQuoteIsAFailure is the postcondition, and it is the whole
// point: the text arrives either way, so only the quote distinguishes a reply
// from an ordinary message — and the sender cannot see the difference.
//
// Confirmed live: with the quote option removed, this fires.
func TestAReplyWithoutAQuoteIsAFailure(t *testing.T) {
	compressClock(t)
	p := &replyDouble{
		resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now(),
		carriesQuote: false,
	}
	_, err := replyer(p)("ORIG1")
	if !errors.Is(err, ErrNotAReply) {
		t.Fatalf("got %v, want ErrNotAReply", err)
	}
}

func TestAReplyThatCarriesItsQuoteSucceeds(t *testing.T) {
	compressClock(t)
	p := &replyDouble{
		resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now(),
		carriesQuote: true,
	}
	got, err := replyer(p)("ORIG1")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if got.ID.ID != "REPLY1" {
		t.Fatalf("verified the wrong message: %+v", got)
	}
}

// TestTheQuoteIsTheMessageNotTheBuiltObject. createQuotedMsgObj looked like the
// way to build it and is not: its source requires quotedStanzaID and returns
// null without one, because it converts a message that ALREADY IS a reply into
// its quoted object. Passing the target there returned null against the live
// account, which is how the misreading was caught.
func TestTheQuoteIsTheMessageNotTheBuiltObject(t *testing.T) {
	compressClock(t)
	p := &replyDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now(), carriesQuote: true}
	if _, err := replyer(p)("ORIG1"); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if strings.Contains(p.lastScript, "createQuotedMsgObj(") {
		t.Fatal("the script calls createQuotedMsgObj, which returns null for a " +
			"message that is not itself a reply")
	}
	if !strings.Contains(p.lastScript, "quotedMsg:") {
		t.Fatal("the quote is not passed through the quotedMsg option")
	}
}

func TestAQuotedMessageThatIsNotLoadedIsItsOwnError(t *testing.T) {
	compressClock(t)
	p := &replyDouble{failStage: "quote", failWhy: "NOT_LOADED"}
	if _, err := replyer(p)("ORIG1"); !errors.Is(err, ErrNoQuoted) {
		t.Fatalf("got %v, want ErrNoQuoted: replying to something the page has not "+
			"loaded is not possible, and sending anyway would produce an ordinary "+
			"message that silently is not a reply", err)
	}
}

func TestABlankQuotedIDNeverReachesThePage(t *testing.T) {
	compressClock(t)
	p := &replyDouble{}
	if _, err := replyer(p)("  "); !errors.Is(err, ErrNoQuoted) {
		t.Fatalf("got %v, want ErrNoQuoted", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to reply %d time(s) with no quoted message", p.kicks)
	}
}

func TestCancelledContextRepliesToNothing(t *testing.T) {
	compressClock(t)
	p := &replyDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now(), carriesQuote: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Reply(ctx, engine.NewRunner(), p.eval, phoneJID, "ORIG1", "x", "t/reply"); err == nil {
		t.Fatal("a cancelled context sent a reply")
	}
	if p.kicks != 0 {
		t.Fatalf("replied %d time(s) for a caller that had given up", p.kicks)
	}
}

// TestAStalledReplyIsItsOwnFailure. The page accepting the kick and never
// settling is not a refusal, and a caller that could not tell them apart would
// retry a send that may already be in flight — the one thing a message API must
// not encourage.
func TestAStalledReplyIsItsOwnFailure(t *testing.T) {
	compressClock(t)
	p := &stallingReplyDouble{}
	_, err := Reply(context.Background(), engine.NewRunner(), p.eval,
		phoneJID, "ORIG1", "x", "t/reply")
	if !errors.Is(err, ErrDispatch) {
		t.Fatalf("got %v, want ErrDispatch", err)
	}
	if !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("a stall must be distinguishable from a refusal: %v", err)
	}
}

type stallingReplyDouble struct{}

func (p *stallingReplyDouble) eval(ctx context.Context, expr string, out *string) error {
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

	if strings.Contains(expr, "const s = window[") {
		*out = `{"stage":"pending","ok":false,"why":""}`
		return nil
	}
	*out = `{"started":true}`
	return nil
}

// TestABlankRecipientNeverReachesThePage closes the last refusal path: a reply
// with no destination is a caller mistake, not a send to attempt.
func TestABlankRecipientNeverReachesThePage(t *testing.T) {
	compressClock(t)
	p := &replyDouble{}
	if _, err := Reply(context.Background(), engine.NewRunner(), p.eval,
		"   ", "ORIG1", "x", "t/reply"); !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked to reply %d time(s) with no recipient", p.kicks)
	}
}
