package liveness

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// socketDouble imitates ONE REAL RULE, measured in H116: after a reconnect the
// socket spends about 450ms in OPENING and then returns to CONNECTED. A double
// that flipped straight back to CONNECTED would be MORE settled than the world
// and would let a verifier that certifies its own starting state pass green.
type socketDouble struct {
	// openingReads is how many state reads answer OPENING after the reconnect.
	// Zero reproduces a page that never visibly transitions, which is what the
	// 500ms sampler saw and what the fast path must still cope with.
	openingReads int
	// stuck keeps the socket in OPENING forever.
	stuck bool
	// refuseWith makes the page answer something other than the ack, and
	// refuseSet says so — because the EMPTY answer is itself one of the cases
	// that must be refused, and a bare empty string could not express it.
	refuseWith string
	refuseSet  bool

	reconnects int
	reads      int
}

func (d *socketDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "reconnect()") {
		d.reconnects++
		if d.refuseSet {
			*out = d.refuseWith
			return nil
		}
		*out = resetAck
		return nil
	}
	// A state read.
	if d.reconnects == 0 {
		*out = string(spa.SocketStateConnected)
		return nil
	}
	d.reads++
	if d.stuck || d.reads <= d.openingReads {
		*out = string(spa.SocketStateOpening)
		return nil
	}
	*out = string(spa.SocketStateConnected)
	return nil
}

func fastClock(t *testing.T) {
	t.Helper()
	ob, ot := ResetBudget, ResetTick
	ResetBudget, ResetTick = 300*time.Millisecond, 2*time.Millisecond
	t.Cleanup(func() { ResetBudget, ResetTick = ob, ot })
}

func reset(d *socketDouble) (ResetOutcome, error) {
	return Reset(context.Background(), engine.NewRunner(), d.eval, "t")
}

// THE HAPPY PATH: the socket leaves and comes back, and both facts are reported.
func TestAResetThatTransitsAndSettles(t *testing.T) {
	fastClock(t)
	d := &socketDouble{openingReads: 3}
	got, err := reset(d)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !got.Settled {
		t.Error("Settled is false for a socket that came back")
	}
	if !got.SawOpening {
		t.Error("SawOpening is false although the socket was seen in OPENING")
	}
	if got.Before != spa.SocketStateConnected {
		t.Errorf("Before = %q; the state at reset time was dropped", got.Before)
	}
	if d.reconnects != 1 {
		t.Errorf("reconnected %d times", d.reconnects)
	}
}

// A SOCKET THAT NEVER COMES BACK IS ITS OWN ERROR, distinct from a refusal: the
// session is now worse than before the reset, and a caller must be able to act
// on that rather than on a generic failure.
func TestASocketThatNeverSettlesIsItsOwnError(t *testing.T) {
	fastClock(t)
	d := &socketDouble{stuck: true}
	got, err := reset(d)
	if !errors.Is(err, ErrNeverSettled) {
		t.Fatalf("err = %v, want ErrNeverSettled", err)
	}
	if errors.Is(err, ErrResetRefused) {
		t.Error("a socket that did not return is being reported as a refused reset")
	}
	if got.Settled {
		t.Error("Settled is true on the error path")
	}
	if !strings.Contains(err.Error(), "OPENING") {
		t.Errorf("the error does not report the last state seen: %v", err)
	}
}

// THE VERIFIER MUST NOT CERTIFY THE STATE IT STARTED FROM.
//
// The socket is CONNECTED before the reset, so a verifier that accepts the first
// CONNECTED it reads would report success without the reconnect having done
// anything at all — including if reconnect were a no-op or the module renamed.
// This is the measured case: with a 500ms sampler, EVERY read is CONNECTED.
func TestAnImmediateConnectedDoesNotCertifyItself(t *testing.T) {
	fastClock(t)
	d := &socketDouble{openingReads: 0} // never visibly transitions
	got, err := reset(d)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got.SawOpening {
		t.Fatal("SawOpening is true although the double never answered OPENING")
	}
	// It still settles — but only after waiting past the point where the answer
	// could be the state it began in.
	if !got.Settled {
		t.Error("a socket that stayed CONNECTED should still settle")
	}
	if d.reads < 4 {
		t.Errorf("the verifier accepted after %d reads; it certified the state it "+
			"started from", d.reads)
	}
}

// A PAGE THAT DID NOT RECONNECT MUST NOT READ AS SUCCESS. An empty answer is
// what an interrupted evaluation produces, which is why the ack is a string.
func TestAPageThatDidNotReconnectIsRefused(t *testing.T) {
	fastClock(t)
	for _, answer := range []string{"", "no-reconnect", "threw", "true"} {
		d := &socketDouble{refuseWith: answer, refuseSet: true}
		if _, err := reset(d); !errors.Is(err, ErrResetRefused) {
			t.Errorf("answer %q gave err = %v, want ErrResetRefused", answer, err)
		}
	}
}

// THE RESET AND THE VERIFY MUST REACH THE SAME SOCKET. Resolving it differently
// in the two scripts would verify an object the reset never touched.
func TestTheResetResolvesTheSocketTheSameWayTheReadDoes(t *testing.T) {
	for _, frag := range []string{"WAWebSocketModel", "m.Socket || m.default || m"} {
		if !strings.Contains(resetScript, frag) {
			t.Errorf("the reset script does not resolve the socket via %q", frag)
		}
		if !strings.Contains(spa.SocketStateReadExpr, frag) {
			t.Errorf("the read expression does not resolve the socket via %q", frag)
		}
	}
}

func TestACancelledCallerNeverReconnects(t *testing.T) {
	fastClock(t)
	d := &socketDouble{openingReads: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Reset(ctx, engine.NewRunner(), d.eval, "t"); err == nil {
		t.Fatal("a cancelled context produced a reset")
	}
	if d.reconnects != 0 {
		t.Errorf("reconnected %d times for a caller that had given up", d.reconnects)
	}
}

func TestTheOutcomeRenderingCarriesNoIdentity(t *testing.T) {
	o := ResetOutcome{Settled: true, SawOpening: true, Before: spa.SocketStateConnected}
	if s := o.String(); !strings.Contains(s, "settled=true") {
		t.Errorf("the rendering lost the postcondition: %s", s)
	}
}
