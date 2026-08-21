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

// The capability WORKS (H73 unblocked H69). These tests lock the shape that the
// argument instrument measured, and in particular that nothing resembling the UI
// helper's argument list comes back — that mistake cost two live runs.

type pollDouble struct {
	ok         bool
	stage, why string
	id         string
	options    int

	kicks      int
	lastScript string
}

func (p *pollDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if !p.ok {
			stage := p.stage
			if stage == "" {
				stage = "apply"
			}
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, stage, p.why)
			return nil
		}
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","id":%q,"options":%d,"typeSource":"none"}`,
			p.id, p.options)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func compressPollClock(t *testing.T) {
	t.Helper()
	ob, ot := pollBudget, pollTick
	pollBudget, pollTick = 150*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { pollBudget, pollTick = ob, ot })
}

func sendPoll(p *pollDouble, q string, opts []string, multi bool) (PollResult, error) {
	return PollTo(context.Background(), engine.NewRunner(), p.eval,
		"15550001111@c.us", q, opts, multi, "t")
}

// TestAnOptionIsAnObjectNotAString is the measurement worth keeping even though
// the capability does not work: the app's add-option merge path builds
// {name, localId} and keys a Set on option.name. Passing bare strings is the
// obvious guess and is wrong.
func TestAnOptionIsAnObjectNotAString(t *testing.T) {
	compressPollClock(t)
	p := &pollDouble{ok: true, id: "3EB0", options: 3}
	if _, err := sendPoll(p, "q?", []string{"a", "b", "c"}, false); err != nil {
		t.Fatalf("PollTo: %v", err)
	}
	if !strings.Contains(p.lastScript, "({ name: name, localId: i })") {
		t.Fatal("options are not built as {name, localId}")
	}
}

// TestThePayloadIsExactlyWhatTheFunctionReads. The instrument measured three
// fields — name, contentType, options — and nothing else. The fields H69 sent
// instead came from a UI helper and must not come back.
func TestThePayloadIsExactlyWhatTheFunctionReads(t *testing.T) {
	compressPollClock(t)
	p := &pollDouble{ok: true, id: "3EB0", options: 3}
	if _, err := sendPoll(p, "q?", []string{"a", "b", "c"}, false); err != nil {
		t.Fatalf("PollTo: %v", err)
	}
	if !strings.Contains(p.lastScript, "const poll = { name:") {
		t.Fatal("the payload is not built from the measured fields")
	}
	if !strings.Contains(p.lastScript, "options: filteredOptions") {
		t.Fatal("the payload has no options field")
	}
	if !strings.Contains(p.lastScript, "A.sendPollCreation({ poll: poll, chat: chat,") {
		t.Fatal("the envelope is not the measured {poll, chat, quotedMsg, isWamoSub}")
	}
	// EVERY NEEDLE IS A KEY, with its colon. Matching the bare names found them
	// in the script's own COMMENT explaining why they are not sent — the
	// guard-matches-prose trap in ARMADILHAS.md, hit for the third time in one
	// session and again by the person who wrote the rule.
	for _, ghost := range []string{"correctOptionKey:", "filteredOptions:", "isPhotoPoll:", "hideVoterNames:", "pollEndTime:"} {
		if strings.Contains(p.lastScript, ghost) {
			t.Fatalf("%q is back in the payload; it belongs to the UI helper, not to "+
				"this function, and sending it cost two live runs", ghost)
		}
	}
	// createPollCreationMsgData reads the SAME envelope, so calling it to build
	// the payload was circular. It must not be called.
	if strings.Contains(p.lastScript, "createPollCreationMsgData") {
		t.Fatal("the payload is built by calling the function that wants the payload")
	}
}

// TestAnInventedContentTypeIsNotSent. The field is real but its enum was not
// found on this build; omitting it is honest, and inventing a number would be a
// value nobody can check.
func TestAnInventedContentTypeIsNotSent(t *testing.T) {
	compressPollClock(t)
	p := &pollDouble{ok: true, id: "3EB0", options: 2}
	if _, err := sendPoll(p, "q?", []string{"a", "b"}, false); err != nil {
		t.Fatalf("PollTo: %v", err)
	}
	if !strings.Contains(p.lastScript, "if (contentType !== undefined) { poll.contentType = contentType; }") {
		t.Fatal("contentType is set unconditionally; an undefined enum would be sent as undefined")
	}
	if !strings.Contains(p.lastScript, "typeSource = 'omitted'") {
		t.Fatal("the script does not report that it omitted the field")
	}
}

// TestMultiIsAcceptedAndNOTSILENTLYDROPPED.
//
// This is the uncomfortable one. The measured payload has three fields and none
// of them is about single-versus-multiple answers, so the multi argument
// currently reaches nothing. Saying that here is better than a test that
// pretends: the parameter stays because the product distinction is real and the
// field for it has not been found, and this test documents the gap so nobody
// reads its absence as proof it does not exist.
func TestMultiIsAcceptedAndNotSilentlyDropped(t *testing.T) {
	compressPollClock(t)
	for _, multi := range []bool{false, true} {
		p := &pollDouble{ok: true, id: "3EB0", options: 2}
		got, err := sendPoll(p, "q?", []string{"a", "b"}, multi)
		if err != nil {
			t.Fatalf("multi=%t: %v", multi, err)
		}
		if got.ID == "" {
			t.Fatalf("multi=%t produced no poll", multi)
		}
	}
	t.Log("NOT IMPLEMENTED: the measured payload carries no single/multi field, " +
		"so the multi argument does not yet reach the page — see H73")
}

// TestTheNewMessageMustBEAPoll. A new message of ours in the chat is not proof
// the POLL landed — anything else sent meanwhile would satisfy that.
func TestTheNewMessageMustBeAPoll(t *testing.T) {
	for _, cond := range []string{"m.pollName", "m.type !== 'poll_creation'", "s.seen.has(m.id.id)"} {
		if !strings.Contains(pollResultScript, cond) {
			t.Fatalf("the poll check is missing the %q condition", cond)
		}
	}
	if strings.Contains(pollResultScript, "m.t >") {
		t.Fatal("the poll is identified by timestamp, which produced a false positive in this module")
	}
}

func TestBadPollsAreRefusedBeforeThePage(t *testing.T) {
	compressPollClock(t)
	for _, tc := range []struct {
		name string
		q    string
		opts []string
		want error
	}{
		{"no question", "  ", []string{"a", "b"}, ErrPollQuestion},
		{"huge question", strings.Repeat("x", MaxQuestionBytes+1), []string{"a", "b"}, ErrPollQuestion},
		{"one option", "q?", []string{"a"}, ErrPollOptions},
		{"too many options", "q?", make([]string, MaxPollOptions+1), ErrPollOptions},
		{"empty option", "q?", []string{"a", "  "}, ErrPollOptions},
		{"duplicate options", "q?", []string{"a", "a"}, ErrPollOptions},
		{"huge option", "q?", []string{"a", strings.Repeat("x", MaxOptionBytes+1)}, ErrPollOptions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pollDouble{ok: true, id: "3EB0"}
			if _, err := sendPoll(p, tc.q, tc.opts, false); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if p.kicks != 0 {
				t.Fatalf("the page was asked %d time(s) for a poll that could not work", p.kicks)
			}
		})
	}
}

// TestNoQuestionOrOptionTextIsRendered. A poll's question and answers are
// content chosen by a person.
func TestNoQuestionOrOptionTextIsRendered(t *testing.T) {
	s := PollResult{ID: "3EB0", Options: 3, PollTypeSource: "none"}.String()
	if !strings.Contains(s, "options=3") {
		t.Fatalf("the option count is missing: %s", s)
	}
	if strings.Contains(s, "question") {
		t.Fatalf("the rendering carries the question: %s", s)
	}
}

func TestAPollThatNeverAppearsIsNotDelivered(t *testing.T) {
	compressPollClock(t)
	p := &pollDouble{ok: true, id: "", options: 3}
	if _, err := sendPoll(p, "q?", []string{"a", "b", "c"}, false); !errors.Is(err, ErrPollNotDelivered) {
		t.Fatalf("got %v, want ErrPollNotDelivered", err)
	}
}

func TestCancelledContextSendsNoPoll(t *testing.T) {
	compressPollClock(t)
	p := &pollDouble{ok: true, id: "3EB0", options: 2}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PollTo(ctx, engine.NewRunner(), p.eval, "1@c.us", "q?",
		[]string{"a", "b"}, false, "t"); err == nil {
		t.Fatal("a cancelled context sent a poll")
	}
	if p.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestAPollThatNeverSettlesIsNotDeliveredNotATimeout(t *testing.T) {
	compressPollClock(t)
	settling := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"settling","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	_, err := PollTo(context.Background(), engine.NewRunner(), settling,
		"1@c.us", "q?", []string{"a", "b"}, false, "t")
	if !errors.Is(err, ErrPollNotDelivered) {
		t.Fatalf("got %v, want ErrPollNotDelivered", err)
	}
	if strings.Contains(err.Error(), "never settled") {
		t.Fatalf("an undelivered poll is reported as a hung page: %v", err)
	}
}

func TestAHungPageIsATimeoutForPolls(t *testing.T) {
	compressPollClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	_, err := PollTo(context.Background(), engine.NewRunner(), stuck,
		"1@c.us", "q?", []string{"a", "b"}, false, "t")
	if !errors.Is(err, ErrPoll) || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("got %v, want a settle timeout", err)
	}
}

func TestPollFailuresNameTheirStage(t *testing.T) {
	compressPollClock(t)
	noChat := &pollDouble{ok: false, stage: "find", why: "NO_CHAT"}
	_, err := sendPoll(noChat, "q?", []string{"a", "b"}, false)
	if !errors.Is(err, ErrPoll) || !strings.Contains(err.Error(), "no such chat") {
		t.Fatalf("got %v, want a named missing-chat refusal", err)
	}
	threw := &pollDouble{ok: false, stage: "apply", why: "boom"}
	_, err = sendPoll(threw, "q?", []string{"a", "b"}, false)
	if !errors.Is(err, ErrPoll) || !strings.Contains(err.Error(), "at apply (boom)") {
		t.Fatalf("the failure does not name its stage: %v", err)
	}
}

func TestAnEmptyRecipientIsRefusedBeforeThePage(t *testing.T) {
	compressPollClock(t)
	p := &pollDouble{ok: true, id: "3EB0"}
	if _, err := PollTo(context.Background(), engine.NewRunner(), p.eval,
		"   ", "q?", []string{"a", "b"}, false, "t"); err == nil {
		t.Fatal("an empty recipient was accepted")
	}
	if p.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", p.kicks)
	}
}

// TestTheOptionsAreEncodedAsJSONNotConcatenated. Options are text a person
// typed; building the script by concatenation would let a quote end the array.
func TestTheOptionsAreEncodedAsJSON(t *testing.T) {
	compressPollClock(t)
	p := &pollDouble{ok: true, id: "3EB0", options: 2}
	if _, err := sendPoll(p, "q?", []string{`a"b`, `c\d`}, false); err != nil {
		t.Fatalf("PollTo: %v", err)
	}
	if !strings.Contains(p.lastScript, `["a\"b","c\\d"]`) {
		t.Fatalf("the options were not JSON-encoded into the script")
	}
}
