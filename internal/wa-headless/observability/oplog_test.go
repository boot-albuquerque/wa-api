package observability

import (
	"strings"
	"sync"
	"testing"
)

func TestRecordsReturnsACopy(t *testing.T) {
	l := NewOpLog()
	l.Add(OpRecord{Op: "Evaluate", Label: "a", Result: ResultOK})

	got := l.Records()
	got[0].Label = "tampered"

	if l.Records()[0].Label != "a" {
		t.Fatal("Records() handed out the internal slice; a caller serialising the " +
			"log could race with, or rewrite, the history it is reporting")
	}
}

func TestTimeoutsCountsOnlyBlownBudgets(t *testing.T) {
	l := NewOpLog()
	l.Add(OpRecord{Op: "Query", Result: ResultOK})
	l.Add(OpRecord{Op: "Query", Result: ResultError})
	l.Add(OpRecord{Op: "StateProbe", Result: ResultTimeout})
	l.Add(OpRecord{Op: "StateProbe", Result: ResultTimeout})

	if n := l.Timeouts(); n != 2 {
		t.Fatalf("Timeouts() = %d, want 2 — an application error is not a "+
			"timeout, and counting it as one overstates how dead the target is", n)
	}
}

// A driver error can quote whatever the page threw. Truncation is not a
// redaction policy, but an unbounded error string is an unbounded amount of
// somebody's data in a log line.
func TestAddTruncatesErrorText(t *testing.T) {
	l := NewOpLog()
	l.Add(OpRecord{Op: "Evaluate", Result: ResultError, Err: strings.Repeat("x", maxErrTextLen*3)})

	if got := len(l.Records()[0].Err); got != maxErrTextLen {
		t.Fatalf("error text kept %d chars, want %d", got, maxErrTextLen)
	}
}

// Operations on one browser come from more than one goroutine as soon as a
// liveness probe exists alongside a command. Run under -race.
func TestAddIsSafeUnderConcurrency(t *testing.T) {
	l := NewOpLog()
	const writers, each = 8, 50

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < each; j++ {
				l.Add(OpRecord{Op: "Evaluate", Result: ResultOK})
				_ = l.Records()
			}
		}()
	}
	wg.Wait()

	if n := len(l.Records()); n != writers*each {
		t.Fatalf("recorded %d operations, want %d — entries were lost", n, writers*each)
	}
}

func TestSinceIsRelativeToTheLogOrigin(t *testing.T) {
	l := NewOpLog()
	if d := l.Since(l.t0); d != 0 {
		t.Fatalf("Since(origin) = %v, want 0; every record in one log must share one clock", d)
	}
}

// TestRedactPageExceptionKeepsShapeAndDropsMessage is the H1 policy.
//
// The cases are the ones MEASURED against a real browser on 2026-08-19, not
// invented: the first is the only shape that leaked, and the rest are here so
// the redactor cannot pay for that one case by destroying the others.
func TestRedactPageExceptionKeepsShapeAndDropsMessage(t *testing.T) {
	const secret = "SEGREDO-DA-PAGINA-5511999999999"

	cases := []struct {
		name    string
		in      string
		wantOut string // "" means: assert only that the secret is gone
		keep    []string
	}{
		{
			name: "Error message is the leak, and only the message goes",
			in:   `exception "Uncaught" (0:9): Error: ` + secret,
			keep: []string{`exception "Uncaught"`, "(0:9)", "Error"},
		},
		{
			name:    "thrown object carries a type name, which is the driver's word",
			in:      `exception "Uncaught" (0:9): Object`,
			wantOut: `exception "Uncaught" (0:9): Object <redacted: page text>`,
		},
		{
			name: "our own errors are not page text and must survive intact",
			in:   `core: boot failed at not_ready (stopped_via=browser.close): core: page classified "OTHER", want "APP_READY"`,
			wantOut: `core: boot failed at not_ready (stopped_via=browser.close): core: page ` +
				`classified "OTHER", want "APP_READY"`,
		},
		{
			name:    "context errors survive intact",
			in:      "context deadline exceeded",
			wantOut: "context deadline exceeded",
		},
		{
			name: "a wrapped page exception keeps the wrapper and loses the tail",
			in:   `core: navigating to the SPA: exception "Uncaught" (0:9): Error: ` + secret,
			keep: []string{"core: navigating to the SPA", `exception "Uncaught"`},
		},
	}

	for _, c := range cases {
		got := RedactPageException(c.in)
		if strings.Contains(got, secret) {
			t.Errorf("%s: the page message survived redaction: %q", c.name, got)
			continue
		}
		if c.wantOut != "" && got != c.wantOut {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.wantOut)
		}
		for _, k := range c.keep {
			if !strings.Contains(got, k) {
				t.Errorf("%s: redaction removed %q, which is diagnosis and not page text: %q",
					c.name, k, got)
			}
		}
	}
}

// TestRedactionSaysSomethingWasRemoved: an empty tail would read as "the page
// threw nothing", and a reader would stop instead of re-running under the trace
// toggle.
func TestRedactionSaysSomethingWasRemoved(t *testing.T) {
	got := RedactPageException(`exception "Uncaught" (0:9): Error: whatever`)
	if !strings.Contains(got, "redacted") {
		t.Fatalf("redaction is silent: %q. A reader cannot tell a removed message "+
			"from an error that had none", got)
	}
}

// TestRecognisedMarkerWithMalformedTailLosesEverything is the fail-closed case,
// and its scope is exactly what the marker can see.
//
// A first version of this test fed `exception SEGREDO ...` — no quote — and it
// PASSED THROUGH UNREDACTED, because detection anchors on the marker measured
// against the real driver (`exception "`). That failure is kept as the reason
// the residual below is declared instead of assumed away.
func TestRecognisedMarkerWithMalformedTailLosesEverything(t *testing.T) {
	const secret = "SEGREDO-5511999999999"
	// Marker present, but no (line:col) — a shape the policy cannot parse.
	got := RedactPageException(`exception "Uncaught" ` + secret + ` sem posicao`)
	if strings.Contains(got, secret) {
		t.Fatalf("a recognised marker with an unparseable tail kept its text: %q. When the "+
			"shape is unknown is exactly when a redactor must not guess where the safe "+
			"part ends", got)
	}
	if !strings.Contains(got, "redacted") {
		t.Fatalf("the fail-closed path is silent: %q", got)
	}
}

// TestResidualRiskIsBoundedByTruncation states, as an executable claim, what
// the redactor does NOT cover.
//
// Detection anchors on the marker the driver was MEASURED to emit. A page
// exception arriving in some other future format would not be recognised at
// all, and the only thing standing between it and the record is maxErrTextLen —
// a bound on radius, not on content. Two mechanisms that fail differently is
// the whole reason truncation was kept when redaction landed.
func TestResidualRiskIsBoundedByTruncation(t *testing.T) {
	t.Setenv(envTraceToggle, "")
	unknown := "ERRO-EM-FORMATO-FUTURO: " + strings.Repeat("x", 500)
	l := NewOpLog()
	l.Add(OpRecord{Op: "probe", Err: unknown})

	got := l.Records()[0].Err
	if len(got) > maxErrTextLen {
		t.Fatalf("an unrecognised error was recorded at %d chars; truncation is the only "+
			"backstop for shapes the redactor cannot see, and it did not apply", len(got))
	}
}

// TestAddRedactsWhatItRecords asserts the property that IS load-bearing: a page
// message must not reach the record.
//
// It was first written as TestAddRedactsBeforeTruncating, claiming the ORDER
// mattered. The negative control that inverted the order PASSED, and that is
// the whole reason this comment says something different now: the exception
// marker sits at the start of the string and truncation cuts the end, so both
// orders redact. A test named for a property the code does not have would have
// been a false guarantee — the class of thing this module spent a week finding.
func TestAddRedactsWhatItRecords(t *testing.T) {
	t.Setenv(envTraceToggle, "")
	secret := strings.Repeat("A", 40) + "-SEGREDO-" + strings.Repeat("B", 40)
	l := NewOpLog()
	l.Add(OpRecord{Op: "probe", Err: `exception "Uncaught" (0:9): Error: ` + secret})

	recs := l.Records()
	if len(recs) != 1 {
		t.Fatalf("got %d records", len(recs))
	}
	if strings.Contains(recs[0].Err, "SEGREDO") {
		t.Fatalf("the page message reached the record: %q", recs[0].Err)
	}
}

// TestTraceToggleKeepsTheFullText is the escape hatch, asserted so nobody
// removes it as dead code. Debugging a page error sometimes needs the message;
// the policy is about what is recorded BY DEFAULT.
func TestTraceToggleKeepsTheFullText(t *testing.T) {
	t.Setenv(envTraceToggle, "1")
	const secret = "SEGREDO-DE-DIAGNOSTICO"
	l := NewOpLog()
	l.Add(OpRecord{Op: "probe", Err: `exception "Uncaught" (0:9): Error: ` + secret})

	recs := l.Records()
	if !strings.Contains(recs[0].Err, secret) {
		t.Fatalf("under %s the full text must survive, or the toggle is useless for "+
			"the debugging it exists for: %q", envTraceToggle, recs[0].Err)
	}
}
