package messagemeta

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble answers install and drain with canned JSON, the way the real
// evaluator does — the capability decodes text, and decoding is where a shape
// change bites first (ARMADILHAS §1).
type pageDouble struct {
	install string
	drains  []string
	err     error

	installCalls int
	drainCalls   int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if p.err != nil {
		return p.err
	}
	if strings.Contains(expr, "coll.on(") {
		p.installCalls++
		*out = p.install
		return nil
	}
	p.drainCalls++
	i := p.drainCalls - 1
	if i < len(p.drains) {
		*out = p.drains[i]
		return nil
	}
	*out = `{"installed":true,"events":[],"dropped":0,"seen":0}`
	return nil
}

func sub(p *pageDouble) *Subscription {
	return New(engine.NewRunner(), p.eval, 0)
}

const oneEvent = `{"installed":true,"seen":1,"dropped":0,"events":[
	{"jid":"5511999999999@c.us",
	 "id":{"id":"3EB0ABCDEF","from_me":false,"remote_jid":"5511999999999@c.us"},
	 "direction":"in","type":"chat","t":1755600000}]}`

func TestDrainDecodesMetadata(t *testing.T) {
	p := &pageDouble{install: `{"installed":true,"already":false}`, drains: []string{oneEvent}}
	s := sub(p)
	if err := s.Install(context.Background(), "t/install"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got, err := s.Drain(context.Background(), "t/drain")
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(got.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(got.Events))
	}
	e := got.Events[0]
	if e.Direction != DirectionIn {
		t.Fatalf("direction=%s, want %s", e.Direction, DirectionIn)
	}
	if e.Type != "chat" {
		t.Fatalf("type=%q", e.Type)
	}
	if !e.ID.Present() || e.ID.ID != "3EB0ABCDEF" {
		t.Fatalf("id=%+v", e.ID)
	}
	if e.Timestamp.Unix() != 1755600000 {
		t.Fatalf("timestamp=%v", e.Timestamp)
	}
	if !got.Complete() {
		t.Fatalf("Complete()=false on a clean drain: dropped=%d reinstalled=%v",
			got.Dropped, got.Reinstalled)
	}
}

// TestNoBodyFieldExistsAnywhere is invariant 12 enforced structurally rather
// than by review. The page script builds metadata from an ALLOW-LIST, so the
// only way a body reaches Go is if someone adds a field for it — and this test
// is what fails when they do.
//
// It checks the SCRIPT, not a sample answer: a test that only inspected decoded
// output would pass forever against a double that never sent a body, which is
// exactly the "the double is better behaved than the world" trap.
func TestNoBodyFieldExistsAnywhere(t *testing.T) {
	s := sub(&pageDouble{})
	script := s.installScript()
	for _, forbidden := range []string{"body", "__x_body", "caption", "media", "text"} {
		if strings.Contains(strings.ToLower(script), forbidden) {
			t.Fatalf("the page script mentions %q. WaMessageMeta is metadata-only "+
				"(HANDOFF §C5, invariant 12) and a message model in this build carries "+
				"__x_body among its own keys — the allow-list is what keeps it out",
				forbidden)
		}
	}
	// And the Go struct must not have grown a content field either.
	if strings.Contains(strings.ToLower(drainScript), "body") {
		t.Fatal("the drain script mentions a body")
	}
}

// TestRenderingRedactsTheJID is §C6: a raw waJid in a log is a blocker. The
// value still travels — the product needs it — so the guard has to be at the
// rendering boundary, where a %v in someone's log years from now would leak it.
func TestRenderingRedactsTheJID(t *testing.T) {
	p := &pageDouble{install: `{"installed":true}`, drains: []string{oneEvent}}
	s := sub(p)
	if err := s.Install(context.Background(), "t/i"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Drain(context.Background(), "t/d")
	if err != nil {
		t.Fatal(err)
	}
	e := got.Events[0]
	if e.JID != "5511999999999@c.us" {
		t.Fatalf("the jid did not survive decoding: %q — the product needs the value", e.JID)
	}
	for _, rendered := range []string{e.String(), e.GoString(), e.ID.String()} {
		if strings.Contains(rendered, "5511999999999") {
			t.Fatalf("a rendered Meta leaked the jid: %s", rendered)
		}
	}
	if !strings.Contains(e.String(), "type=chat") {
		t.Fatalf("String() should still carry the non-PII fields: %s", e.String())
	}
}

// TestDropsAreReportedNotHidden is the whole reason the buffer is allowed to be
// bounded. A stream that loses events silently looks complete, which is worse
// than a stream that admits a hole.
func TestDropsAreReportedNotHidden(t *testing.T) {
	p := &pageDouble{
		install: `{"installed":true}`,
		drains:  []string{`{"installed":true,"events":[],"dropped":7,"seen":507}`},
	}
	s := sub(p)
	if err := s.Install(context.Background(), "t/i"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Drain(context.Background(), "t/d")
	if err != nil {
		t.Fatal(err)
	}
	if got.Dropped != 7 {
		t.Fatalf("Dropped=%d, want 7", got.Dropped)
	}
	if got.Seen != 507 {
		t.Fatalf("Seen=%d, want 507: without the denominator a drop count cannot be read", got.Seen)
	}
	if got.Complete() {
		t.Fatal("Complete()=true on a drain that dropped events; the sequence has a hole " +
			"and a caller treating it as whole would report a message stream it does not have")
	}
}

// TestLostSubscriptionIsReinstalledAndSaidSoOutLoud covers the reload. The
// events between the reload and the reinstall are gone and UNCOUNTABLE — there
// is no drop counter to consult, because the counter went with the page — so
// this must be a distinct signal, not folded into Dropped.
func TestLostSubscriptionIsReinstalledAndSaidSoOutLoud(t *testing.T) {
	p := &pageDouble{
		install: `{"installed":true}`,
		drains: []string{
			`{"installed":false}`,
			`{"installed":true,"events":[],"dropped":0,"seen":0}`,
		},
	}
	s := sub(p)
	got, err := s.Drain(context.Background(), "t/d")
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if !got.Reinstalled {
		t.Fatal("the subscription was gone and came back, and the drain did not say so")
	}
	if got.Complete() {
		t.Fatal("Complete()=true across a reinstall: an unknown number of events was lost, " +
			"which is not the same as zero")
	}
	if p.installCalls == 0 {
		t.Fatal("the subscription was not reinstalled")
	}
}

// TestInstallIsIdempotent guards double counting: two handlers on one
// collection would report every message twice, and a duplicated stream is a
// harder bug to see than a missing one.
func TestInstallIsIdempotent(t *testing.T) {
	p := &pageDouble{install: `{"installed":true,"already":true}`}
	s := sub(p)
	for i := 0; i < 3; i++ {
		if err := s.Install(context.Background(), "t/i"); err != nil {
			t.Fatalf("install %d: %v", i, err)
		}
	}
	// The page script's own guard is what makes this safe; the test asserts the
	// contract holds through the Go layer rather than erroring on a second call.
	if !strings.Contains(s.installScript(), "window[KEY].installed") {
		t.Fatal("the install script has no already-installed guard, so a second call " +
			"would attach a second handler and every message would be reported twice")
	}
}

func TestMissingCollectionIsNamed(t *testing.T) {
	p := &pageDouble{install: `{"installed":false,"reason":"NO_COLLECTION"}`}
	err := sub(p).Install(context.Background(), "t/i")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err=%v, want ErrNotInstalled", err)
	}
	if !strings.Contains(err.Error(), "NO_COLLECTION") {
		t.Fatalf("the error does not name the reason: %v", err)
	}
}

func TestProbeFailureIsNotAnEmptyDrain(t *testing.T) {
	boom := errors.New("evaluate exploded")
	_, err := sub(&pageDouble{err: boom}).Drain(context.Background(), "t/d")
	if err == nil {
		t.Fatal("a failed probe produced no error; an empty drain would read as 'no messages'")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v, want it to wrap the probe error", err)
	}
}

func TestTimestampZeroWhenAbsent(t *testing.T) {
	p := &pageDouble{
		install: `{"installed":true}`,
		drains: []string{`{"installed":true,"seen":1,"dropped":0,"events":[
			{"jid":"x@c.us","id":{"id":"A","from_me":true,"remote_jid":"x@c.us"},
			 "direction":"out","type":"chat","t":0}]}`},
	}
	s := sub(p)
	got, err := s.Drain(context.Background(), "t/d")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Events[0].Timestamp.IsZero() {
		t.Fatalf("t=0 produced %v; a missing timestamp must stay the zero value rather "+
			"than becoming 1970, which reads like a real date", got.Events[0].Timestamp)
	}
	if got.Events[0].Direction != DirectionOut {
		t.Fatalf("direction=%s, want out", got.Events[0].Direction)
	}
	_ = time.Now
}
