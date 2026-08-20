package observability

// OpLog answers one question, and it is the question every failed run of the
// study asked first: WHERE did it stop.
//
// A stack that drives a browser fails by stopping, not by erroring. The 24
// minute hang of phase 6 produced no log line at all until every operation was
// recorded with its promised budget beside its actual duration; then the same
// failure printed twelve entries that named the stage and the class. That is
// the whole design intent — the record exists so that silence becomes evidence.
//
// Study origin: scripts/chromium-study/p4c_deadline.go (phase 4C, sections 3-6).

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Result classifies the outcome of one operation. The three values are kept
// distinct because collapsing them is exactly the misdiagnosis this package
// exists to prevent: "the target answered with an error" and "the target did
// not answer" are different claims about a session's health.
type Result string

const (
	ResultOK       Result = "ok"
	ResultTimeout  Result = "deadline_exceeded"
	ResultError    Result = "error"
	envTraceToggle        = "WA_HEADLESS_OP_TRACE"
	// maxErrTextLen bounds what an operation error contributes to the record.
	// It bounds the blast RADIUS; the redaction below bounds the CONTENT, and
	// both are kept because they fail differently: truncation still holds if a
	// page error arrives in a shape the redactor does not recognise.
	maxErrTextLen = 200
)

// OpRecord is one executed operation: what it promised, and what it delivered.
//
// No field carries page content by design. Class, label, timings and outcome
// are enough to locate where a run stopped without recording anything about the
// account under test.
type OpRecord struct {
	Op         string `json:"operation"`
	Label      string `json:"label,omitempty"`
	StartMS    int64  `json:"start_ms"`
	DurationMS int64  `json:"duration_ms"`
	DeadlineMS int64  `json:"deadline_ms"`
	Result     Result `json:"result"`
	Err        string `json:"error,omitempty"`
}

// OpLog accumulates the operation history of one session or one run.
// It is safe for concurrent use: operations on a single browser are issued from
// more than one goroutine as soon as a liveness probe exists.
type OpLog struct {
	mu      sync.Mutex
	t0      time.Time
	records []OpRecord
	echo    bool
}

// NewOpLog starts a log whose timestamps are relative to now.
func NewOpLog() *OpLog {
	return &OpLog{t0: time.Now(), echo: os.Getenv(envTraceToggle) != ""}
}

// Since is the elapsed time from the log's origin, which is what StartMS
// measures. Callers building a record use it so that every entry in one log
// shares one clock.
func (l *OpLog) Since(t time.Time) time.Duration { return t.Sub(l.t0) }

// Add appends a record, redacting and then truncating its error text.
//
// THE ORDER DOES NOT MATTER FOR SAFETY, and saying so is a correction: this
// comment first claimed that truncating first would keep half a page message.
// The negative control that inverted the order PASSED, which is what exposed
// the claim as wrong — the exception marker sits at the START of the string and
// truncation cuts the END, so the redactor recognises the shape either way.
//
// Redaction still runs first, for a smaller reason worth stating honestly: it
// makes the recorded text the redacted one rather than a truncated-then-redacted
// one, so what a reader sees is what the policy produced.
func (l *OpLog) Add(r OpRecord) {
	if !l.echo {
		r.Err = RedactPageException(r.Err)
	}
	if len(r.Err) > maxErrTextLen {
		r.Err = r.Err[:maxErrTextLen]
	}
	l.mu.Lock()
	l.records = append(l.records, r)
	l.mu.Unlock()
	if l.echo {
		fmt.Fprintf(os.Stderr, "  op=%-14s %-24s %6dms / %6dms  %s%s\n",
			r.Op, r.Label, r.DurationMS, r.DeadlineMS, r.Result, errSuffix(r.Err))
	}
}

func errSuffix(e string) string {
	if e == "" {
		return ""
	}
	return "  (" + e + ")"
}

// Records returns a copy, so a caller serialising the log cannot race with an
// operation still being recorded.
func (l *OpLog) Records() []OpRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]OpRecord, len(l.records))
	copy(out, l.records)
	return out
}

// Timeouts counts operations that blew their budget — the number that says
// whether a run was conducted or merely survived.
func (l *OpLog) Timeouts() int {
	n := 0
	for _, r := range l.Records() {
		if r.Result == ResultTimeout {
			n++
		}
	}
	return n
}

// pageExceptionPrefix is how the driver reports something the PAGE threw.
//
// MEASURED against a real browser on 2026-08-19, not inferred from the driver's
// source. Four shapes were produced deliberately and their error strings read:
//
//	throw new Error("SECRET")  -> exception "Uncaught" (0:9): Error: SECRET
//	throw {chat: "SECRET"}     -> exception "Uncaught" (0:9): Object
//	returning non-JSON text    -> no error at all
//	referencing a missing name -> exception "Uncaught" (0:20): ReferenceError: ...
//
// Only the FIRST leaks: an Error's message is free text the page chose, and on
// web.whatsapp.com that page is holding an account's conversations. The others
// carry a type name or a position, which name the driver's world rather than
// the account's.
const pageExceptionPrefix = `exception "`

// pageExceptionShape captures the safe head of a page exception — the marker,
// the source position and the error TYPE — and leaves the message out.
//
// Anchored rather than searched: matching only at the start of the exception
// span means a message that itself contains the word "exception" cannot move
// where the cut happens.
var pageExceptionShape = regexp.MustCompile(
	`exception "[^"]*" \(\d+:\d+\): ?([A-Za-z_$][A-Za-z0-9_$]*)?`)

// redactedMarker is what replaces a page message. It is deliberately visible:
// an empty tail would read as "the page threw nothing", and knowing that
// something was removed is what tells a reader to re-run under the trace
// toggle rather than conclude the error was empty.
const redactedMarker = " <redacted: page text>"

// RedactPageException removes the free-text tail of a driver error that quotes
// what the PAGE threw, keeping the shape that makes the error diagnosable.
//
// It is exported so the policy can be tested and reused rather than
// reimplemented — H6 was one text capture guarded in one place, and the lesson
// this module keeps relearning is that a rule written twice drifts.
//
// The FULL text is still available under WA_HEADLESS_OP_TRACE, which is the
// deliberate escape hatch: debugging a page error sometimes needs the message,
// and the difference between "available when asked for" and "recorded by
// default" is the whole policy (HOUSEKEEP H1, option a).
func RedactPageException(s string) string {
	i := strings.Index(s, pageExceptionPrefix)
	if i < 0 {
		// No page exception in here. Everything this module writes itself —
		// stage names, deadlines, classes — is its own vocabulary, not the
		// account's, and redacting it would cost diagnosis for no gain.
		return s
	}
	head := s[:i]
	rest := s[i:]

	loc := pageExceptionShape.FindStringIndex(rest)
	if loc == nil {
		// A page exception in a shape this policy does not recognise. Drop the
		// whole tail rather than guess: an unrecognised shape is exactly when a
		// redactor must not assume it knows where the safe part ends.
		return head + pageExceptionPrefix + redactedMarker
	}
	return head + rest[:loc[1]] + redactedMarker
}
