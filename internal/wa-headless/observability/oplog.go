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
	// A driver error can quote whatever the page threw, and a page error can
	// quote whatever the page was holding; truncation is a bound on the blast
	// radius, not a redaction policy. The redaction policy is still owed —
	// see internal/wa-headless/HOUSEKEEP.md.
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

// Add appends a record, truncating its error text.
func (l *OpLog) Add(r OpRecord) {
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
