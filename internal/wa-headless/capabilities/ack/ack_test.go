package ack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

func reader(fn func(context.Context, string, *string) error) *Reader {
	return New(engine.NewRunner(), fn)
}

// answer builds an evaluator double. IT HONOURS THE CONTEXT, because the real
// one does: chromedp fails a cancelled context rather than running the script.
// A double that ignored it would be more permissive than production, which is
// the trap ARMADILHAS.md opens with — and here it would make
// TestCancelledContextReadsNothing pass while the page was being touched.
func answer(ok bool, why string, raw int, name string, fromMe bool, source string) func(context.Context, string, *string) error {
	return func(ctx context.Context, _ string, out *string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !ok {
			*out = fmt.Sprintf(`{"ok":false,"why":%q}`, why)
			return nil
		}
		*out = fmt.Sprintf(`{"ok":true,"why":"","ack":%d,"name":%q,"fromMe":%t,"source":%q}`,
			raw, name, fromMe, source)
		return nil
	}
}

// TestTheStateIsMappedByNameNotByNumber is the design decision worth locking.
// A build that renumbers its ack enum must change Status.Raw and nothing else;
// a package that mapped integers would relabel every message silently.
func TestTheStateIsMappedByNameNotByNumber(t *testing.T) {
	// Deliberately absurd numbers with correct names.
	for _, tc := range []struct {
		name string
		raw  int
		want State
	}{
		{"READ", 99, Read},
		{"DELIVERED", -7, Delivered},
		{"SENT", 41, Sent},
		{"PLAYED", 0, Played},
		{"PENDING", 3, Pending},
		{"ERROR", 2, Error},
	} {
		got, err := reader(answer(true, "", tc.raw, tc.name, true, "WAWebAck.ACK")).
			Of(context.Background(), "3EB0", "t")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.State != tc.want {
			t.Fatalf("name %q with raw %d mapped to %s, want %s", tc.name, tc.raw, got.State, tc.want)
		}
		if got.Raw != tc.raw {
			t.Fatalf("the page's own number was not preserved: %s", got)
		}
	}
}

// TestTheEnumIsReadFromThePage. The script looks the number up IN the page's
// enum and reports where the name came from, so a fallback label is
// distinguishable from a measured one.
func TestTheEnumIsReadFromThePage(t *testing.T) {
	script := ackScript("3EB0")
	if !strings.Contains(script, "if (e[k] === ack) { name = k;") {
		t.Fatal("the script does not look the ack up in the page's enum")
	}
	if !strings.Contains(script, "source = 'WAWebAck.' + key") {
		t.Fatal("the script does not report where the name came from")
	}
	if !strings.Contains(script, "let name = '', source = 'fallback';") {
		t.Fatal("the fallback is not labelled as one")
	}

	measured, err := reader(answer(true, "", 3, "READ", true, "WAWebAck.ACK")).
		Of(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	guessed, err := reader(answer(true, "", 3, "READ", true, "fallback")).
		Of(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if measured.EnumSource == guessed.EnumSource {
		t.Fatal("a measured label and a guessed one are indistinguishable")
	}
}

// TestAnUnnamedAckIsUnknownNotPending. The zero value must not be a claim: a
// state nobody could name is not the same as a message waiting to go out.
func TestAnUnnamedAckIsUnknownNotPending(t *testing.T) {
	got, err := reader(answer(true, "", 77, "", true, "fallback")).
		Of(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if got.State != Unknown {
		t.Fatalf("an unnamed ack became %s", got.State)
	}
	if Unknown == Pending {
		t.Fatal("Unknown and Pending are the same value; the zero value is a claim")
	}
	if got.String() == "" || !strings.Contains(got.String(), "state=unknown") {
		t.Fatalf("the rendering hides that nothing could be named: %s", got)
	}
}

// TestTheStatesAreOrderedByProgress, so a caller can ask "at least delivered?".
func TestTheStatesAreOrderedByProgress(t *testing.T) {
	if Error >= Pending || Pending >= Sent || Sent >= Delivered || Delivered >= Read || Read >= Played {
		t.Fatal("the states are not ordered by progress, so comparisons are meaningless")
	}
	if Unknown != 0 {
		t.Fatal("Unknown is not the zero value, so a zero Status claims a state")
	}
}

// TestFromMeIsCarried. An incoming message's ack is about what THIS account
// acknowledged, which is a different question from "did they read mine".
func TestFromMeIsCarried(t *testing.T) {
	got, err := reader(answer(true, "", 3, "READ", false, "WAWebAck.ACK")).
		Of(context.Background(), "3EB0", "t")
	if err != nil {
		t.Fatalf("Of: %v", err)
	}
	if got.FromMe {
		t.Fatal("an incoming message is reported as ours")
	}
	if !strings.Contains(got.String(), "fromMe=false") {
		t.Fatalf("the rendering does not say whose message it is: %s", got)
	}
}

func TestRefusalsAndFailures(t *testing.T) {
	called := false
	if _, err := reader(func(context.Context, string, *string) error {
		called = true
		return nil
	}).Of(context.Background(), "   ", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	if called {
		t.Fatal("the page was asked about an empty id")
	}
	if _, err := reader(answer(false, "NOT_LOADED", 0, "", false, "")).
		Of(context.Background(), "3EB0", "t"); !errors.Is(err, ErrNoMessage) {
		t.Fatalf("got %v, want ErrNoMessage", err)
	}
	if _, err := reader(answer(false, "boom", 0, "", false, "")).
		Of(context.Background(), "3EB0", "t"); !errors.Is(err, ErrAck) {
		t.Fatalf("got %v, want ErrAck", err)
	}
	if _, err := reader(func(_ context.Context, _ string, out *string) error {
		*out = "not json"
		return nil
	}).Of(context.Background(), "3EB0", "t"); err == nil {
		t.Fatal("a non-JSON answer was accepted")
	}
	if _, err := reader(func(context.Context, string, *string) error {
		return errors.New("evaluator down")
	}).Of(context.Background(), "3EB0", "t"); !errors.Is(err, ErrAck) {
		t.Fatalf("got %v, want ErrAck", err)
	}
}

func TestCancelledContextReadsNothing(t *testing.T) {
	called := false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader(func(c context.Context, _ string, _ *string) error {
		if e := c.Err(); e != nil {
			return e
		}
		called = true
		return nil
	}).Of(ctx, "3EB0", "t"); err == nil {
		t.Fatal("a cancelled context still read an ack")
	}
	if called {
		t.Fatal("the page was asked for a caller that had given up")
	}
}
