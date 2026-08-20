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

// pageDouble stands in for the SPA, and it imitates ONE REAL RULE rather than
// being convenient: this build stores messages under the identity the SERVER
// returns, which is a LID, not under the phone number the caller typed.
//
// That is not a guess. Measured 2026-08-20 against the paired lab account by
// walking the whole collection (internal/wa-headless/probe_sendjid_test.go):
// of 399 models in WAWebMsgCollection, 397 carry server "lid", one "c.us" and
// one "g.us". So the double answers the verification query ONLY when asked for
// storedUnder — a double that answered for any jid would be more permissive
// than the world, and would let the exact defect this suite exists to catch
// pass green (ARMADILHAS §1, HOUSEKEEP H34).
type pageDouble struct {
	// resolvedJID is what queryWidExists hands back: the server's identity.
	resolvedJID string
	// omitJID reproduces a page that reports success without an identity.
	omitJID bool
	// failStage / failWhy park a refusal instead of a success.
	failStage string
	failWhy   string

	// storedUnder is the jid the outgoing message is filed under. It is
	// deliberately a SEPARATE field from resolvedJID so a test can make them
	// disagree.
	storedUnder string
	// storedAt is the message's timestamp; zero means "no message at all".
	storedAt time.Time

	// verifyWants records every jid the verification asked about, which is how
	// the tests assert WHICH identity was used rather than only the outcome.
	verifyWants []string
	dispatched  int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	switch {
	case strings.Contains(expr, "getModelsArray"):
		want := between(expr, `const want = "`, `"`)
		p.verifyWants = append(p.verifyWants, want)
		if p.storedAt.IsZero() || want != p.storedUnder {
			*out = `[]`
			return nil
		}
		*out = fmt.Sprintf(
			`[{"jid":%q,"id":{"id":"MSG1","from_me":true,"remote_jid":%q},`+
				`"direction":"out","type":"chat","t":%d}]`,
			p.storedUnder, p.storedUnder, p.storedAt.Unix())
		return nil

	case strings.Contains(expr, "const s = window["):
		if p.failStage != "" {
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, p.failStage, p.failWhy)
			return nil
		}
		if p.omitJID {
			*out = `{"stage":"dispatch","ok":true,"why":""}`
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"dispatch","ok":true,"why":"","jid":%q}`, p.resolvedJID)
		return nil

	default: // the dispatch kick
		p.dispatched++
		*out = `{"started":true}`
		return nil
	}
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	if j := strings.Index(s, b); j >= 0 {
		return s[:j]
	}
	return s
}

// compressClock shrinks the verification budget so the failure paths do not
// each sit out twenty seconds.
func compressClock(t *testing.T) {
	t.Helper()
	oldBudget, oldTick := verifyBudget, verifyTick
	verifyBudget, verifyTick = 150*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { verifyBudget, verifyTick = oldBudget, oldTick })
}

const (
	phoneJID = "5541992421234@c.us"
	lidJID   = "123456789012345@lid"
)

func send(p *pageDouble) (Result, error) {
	return Text(context.Background(), engine.NewRunner(), p.eval, phoneJID, "marker", "t/send")
}

// TestVerificationUsesTheRESOLVEDIdentity is the regression for H34, and it is
// the whole reason this file exists.
//
// The caller passes a PHONE jid. The page resolves it to a LID and files the
// message under the LID. A verifier that reused the caller's jid finds nothing
// and reports ErrUnverified for a send that worked — which is exactly what
// shipped, and what the real closed-loop test caught in the field.
func TestVerificationUsesTheRESOLVEDIdentity(t *testing.T) {
	compressClock(t)
	p := &pageDouble{
		resolvedJID: lidJID,
		storedUnder: lidJID, // the server's identity, NOT phoneJID
		storedAt:    time.Now(),
	}
	res, err := send(p)
	if err != nil {
		t.Fatalf("Text: %v", err)
	}
	if res.ID.ID != "MSG1" {
		t.Fatalf("verified the wrong message: %+v", res)
	}
	// Outcome alone would not pin the defect: assert WHICH identity was asked
	// for, so a future verifier that happens to succeed for the wrong reason
	// still fails here.
	if len(p.verifyWants) == 0 {
		t.Fatal("verification never queried the page")
	}
	for _, w := range p.verifyWants {
		if w != lidJID {
			t.Fatalf("verification asked for %q, want the resolved %q — asking for "+
				"the caller's jid is H34 exactly", w, lidJID)
		}
	}
}

// TestSuccessWithoutAnIdentityIsRefused: a page that reports ok:true but hands
// back no identity leaves verification with nothing to aim at. Treating that as
// success would be the silent success invariant 14 forbids, so it is a refusal.
func TestSuccessWithoutAnIdentityIsRefused(t *testing.T) {
	compressClock(t)
	p := &pageDouble{omitJID: true, storedUnder: lidJID, storedAt: time.Now()}
	_, err := send(p)
	if !errors.Is(err, ErrDispatch) {
		t.Fatalf("got %v, want ErrDispatch", err)
	}
}

// TestNotOnWhatsAppIsErrNoChat keeps the three failures distinguishable. A
// recipient who does not exist is a different problem, with a different repair,
// than a page that refused — and one generic error would hide that.
func TestNotOnWhatsAppIsErrNoChat(t *testing.T) {
	compressClock(t)
	p := &pageDouble{failStage: "resolve", failWhy: "NOT_ON_WHATSAPP"}
	_, err := send(p)
	if !errors.Is(err, ErrNoChat) {
		t.Fatalf("got %v, want ErrNoChat", err)
	}
	if errors.Is(err, ErrDispatch) || errors.Is(err, ErrUnverified) {
		t.Fatalf("an unresolvable recipient must not also read as %v", err)
	}
}

func TestPageRefusalIsErrDispatch(t *testing.T) {
	compressClock(t)
	p := &pageDouble{failStage: "dispatch", failWhy: "boom"}
	_, err := send(p)
	if !errors.Is(err, ErrDispatch) {
		t.Fatalf("got %v, want ErrDispatch", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("the page's reason was dropped: %v", err)
	}
}

// TestNothingAppearingIsErrUnverified is the postcondition itself: the page
// said yes and no message exists.
func TestNothingAppearingIsErrUnverified(t *testing.T) {
	compressClock(t)
	p := &pageDouble{resolvedJID: lidJID} // storedAt zero: nothing was filed
	_, err := send(p)
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("got %v, want ErrUnverified", err)
	}
}

// TestStaleMessagesDoNotVerifyASend guards the freshness bound. The collection
// replays history, so "an outgoing message to this chat exists" is true for
// every chat that ever had one. Without the timestamp bound, every send to a
// known contact would verify instantly — including the ones that failed.
func TestStaleMessagesDoNotVerifyASend(t *testing.T) {
	compressClock(t)
	p := &pageDouble{
		resolvedJID: lidJID,
		storedUnder: lidJID,
		// Older than the clock-skew allowance, so it is unambiguously history.
		storedAt: time.Now().Add(-clockSkewAllowance - time.Hour),
	}
	_, err := send(p)
	if !errors.Is(err, ErrUnverified) {
		t.Fatalf("got %v, want ErrUnverified — a message from an hour ago cannot "+
			"prove a send from a moment ago", err)
	}
}

// TestAlreadyCancelledContextNeverDispatches is the ordering that matters: a
// caller who gave up must not have a real message sent on their behalf.
func TestAlreadyCancelledContextNeverDispatches(t *testing.T) {
	compressClock(t)
	p := &pageDouble{resolvedJID: lidJID, storedUnder: lidJID, storedAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Text(ctx, engine.NewRunner(), p.eval, phoneJID, "marker", "t/cancelled"); err == nil {
		t.Fatal("a cancelled context produced a successful send")
	}
	if p.dispatched != 0 {
		t.Fatalf("dispatched %d time(s) for a caller that had already given up", p.dispatched)
	}
}
