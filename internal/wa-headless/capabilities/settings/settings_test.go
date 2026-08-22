package settings

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble imitates ONE REAL RULE rather than being convenient: the page keeps
// the four categories SEPARATELY, under four differently-named getters. Measured
// 2026-08-22 against the lab account, which held them at mixed values (audio on,
// documents off, photos on, videos off) — so a double that answered the same
// thing for every kind would be more uniform than the world and would let a
// reader that ignores the kind pass green.
type pageDouble struct {
	values map[string]bool
	// refuseWrite makes the setter a no-op, which is what a page that accepted
	// the call and did nothing looks like. THE ZERO VALUE IS THE HEALTHY ONE.
	refuseWrite bool

	answer     string
	lastScript string
	kicks      int
	writes     int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+stateKey) {
		*out = p.answer
		return nil
	}
	p.kicks++
	p.lastScript = expr
	p.answer = p.respond(expr)
	*out = "kicked"
	return nil
}

// answer is what the parked global currently holds.
func (p *pageDouble) respond(expr string) string {
	// A read asks for every getter at once; a write names exactly one setter.
	for kind, acc := range accessors {
		if strings.Contains(expr, acc[1]+"(") {
			return p.doWrite(expr, string(kind), acc[1])
		}
	}
	if strings.Contains(expr, fnSyncSet+"(") {
		return p.doWrite(expr, "sync", fnSyncSet)
	}
	var b strings.Builder
	b.WriteString(`{"ok":true,"auto":{`)
	for i, k := range Kinds {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"` + string(k) + `":` + boolJSON(p.values[string(k)]))
	}
	b.WriteString(`},"sync":` + boolJSON(p.values["sync"]) + `}`)
	return b.String()
}

func (p *pageDouble) doWrite(expr, key, setter string) string {
	before := p.values[key]
	want := strings.Contains(expr, setter+"(true)")
	skipped := before == want
	if !skipped && !p.refuseWrite {
		p.writes++
		p.values[key] = want
	} else if !skipped {
		p.writes++
	}
	after := p.values[key]
	return `{"ok":true,"before":` + boolJSON(before) + `,"after":` + boolJSON(after) +
		`,"skipped":` + boolJSON(skipped) + `}`
}

func boolJSON(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func mgr(p *pageDouble) *Manager { return New(engine.NewRunner(), p.eval) }

func mixed() *pageDouble {
	// The values MEASURED on the lab account, not invented ones.
	return &pageDouble{values: map[string]bool{
		"audio": true, "documents": false, "photos": true, "videos": false, "sync": false,
	}}
}

func withoutComments(script string) string {
	var b strings.Builder
	for _, line := range strings.Split(script, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// EACH CATEGORY IS ITS OWN SETTING, and a reader that returned one value for all
// four would be caught only by a double that keeps them apart — which is why the
// double uses the measured mixed values.
func TestTheFourCategoriesAreReadSeparately(t *testing.T) {
	p := mixed()
	st, err := mgr(p).Read(context.Background(), "t")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for kind, want := range map[Kind]bool{
		KindAudio: true, KindDocuments: false, KindPhotos: true, KindVideos: false,
	} {
		if st.AutoDownload[kind] != want {
			t.Errorf("%s = %t, want %t", kind, st.AutoDownload[kind], want)
		}
	}
	if len(st.AutoDownload) != len(Kinds) {
		t.Errorf("read %d categories, want %d", len(st.AutoDownload), len(Kinds))
	}

	// AND THE SCRIPT MUST ASK FOR EACH ONE, which the checks above cannot see.
	//
	// The double answers from its own map, so pointing every read at
	// getAutoDownloadAudio left this test green — a negative control that did not
	// bite, for the fourth time in this work: whenever the double SUPPLIES the
	// value the assertion examines, the test measures the parsing and the rule
	// lives in the script.
	code := withoutComments(p.lastScript)
	for _, k := range Kinds {
		if !strings.Contains(code, accessors[k][0]+"()") {
			t.Errorf("the read script never calls %s(): that category is not being "+
				"read at all", accessors[k][0])
		}
	}
}

// A WRITE THE PAGE IGNORED IS NOT A SUCCESS. This is invariant 14, and it is the
// exact thing the reference cannot report: its setters end with `return flag`.
func TestAWriteThePageIgnoredIsAnError(t *testing.T) {
	p := mixed()
	p.refuseWrite = true
	_, err := mgr(p).SetAutoDownload(context.Background(), KindDocuments, true, "t")
	if !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
	if !strings.Contains(err.Error(), "still holds") {
		t.Errorf("the error does not say what the page holds: %v", err)
	}
}

// AND THE SCRIPT MUST READ BACK, which the check above cannot see: the double
// supplies `after` ready-made. The property lives in the production.
func TestTheWriteScriptReadsTheValueBack(t *testing.T) {
	p := mixed()
	if _, err := mgr(p).SetAutoDownload(context.Background(), KindVideos, true, "t"); err != nil {
		t.Fatalf("SetAutoDownload: %v", err)
	}
	code := withoutComments(p.lastScript)
	get := accessors[KindVideos][0]
	if strings.Count(code, get+"()") < 2 {
		t.Fatalf("the script calls %s() fewer than twice: it cannot be comparing "+
			"before against after, so the postcondition is decorative", get)
	}
	if !strings.Contains(code, "after:") {
		t.Error("the script parks no read-back value")
	}
}

// A REDUNDANT WRITE IS REPORTED, NOT HIDDEN. H55 measured a redundant request
// being the cause of a failure, so it is skipped — but "did not need to" and
// "wrote it" are different answers and a caller can tell them apart.
func TestARedundantWriteIsSkippedAndSaidSo(t *testing.T) {
	p := mixed() // audio is already true
	got, err := mgr(p).SetAutoDownload(context.Background(), KindAudio, true, "t")
	if err != nil {
		t.Fatalf("SetAutoDownload: %v", err)
	}
	if got.Changed {
		t.Error("Changed is true for a setting that was already there")
	}
	if !got.Now {
		t.Error("Now lost the value the page holds")
	}
	if p.writes != 0 {
		t.Errorf("the page was written %d time(s) for a redundant request", p.writes)
	}

	// And a real move reports Changed.
	got, err = mgr(p).SetAutoDownload(context.Background(), KindDocuments, true, "t")
	if err != nil {
		t.Fatalf("SetAutoDownload: %v", err)
	}
	if !got.Changed {
		t.Error("Changed is false for a write that actually moved the setting")
	}

	// AND THE SCRIPT MUST BE WHAT SKIPS. Same gap as above: the double computes
	// `skipped` itself, so removing the guard from the script left this green.
	code := withoutComments(p.lastScript)
	if !strings.Contains(code, "if (before ===") {
		t.Fatal("the script has no guard against a redundant write; H55 measured a " +
			"redundant request being the CAUSE of a failure")
	}
}

// An unknown category never reaches the page: the page spells the category into
// the function name, so a wrong kind would build a call to a function that does
// not exist and fail late and confusingly.
func TestAnUnknownCategoryNeverReachesThePage(t *testing.T) {
	p := mixed()
	_, err := mgr(p).SetAutoDownload(context.Background(), Kind("stickers"), true, "t")
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("err = %v, want ErrUnknownKind", err)
	}
	if p.kicks != 0 {
		t.Errorf("the page was asked %d time(s) about a category it does not keep", p.kicks)
	}
}

func TestBackgroundSyncMovesAndProvesIt(t *testing.T) {
	p := mixed() // sync is false
	got, err := mgr(p).SetBackgroundSync(context.Background(), true, "t")
	if err != nil {
		t.Fatalf("SetBackgroundSync: %v", err)
	}
	if !got.Now || !got.Changed {
		t.Fatalf("outcome = %s, want a change to true", got)
	}
	p.refuseWrite = true
	p.values["sync"] = false
	if _, err := mgr(p).SetBackgroundSync(context.Background(), true, "t"); !errors.Is(err, ErrNotTaken) {
		t.Fatalf("err = %v, want ErrNotTaken", err)
	}
}

// A cancelled caller never writes.
func TestACancelledCallerNeverWrites(t *testing.T) {
	p := mixed()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := mgr(p).SetAutoDownload(ctx, KindVideos, true, "t"); err == nil {
		t.Fatal("a cancelled context produced a successful write")
	}
	if p.kicks != 0 {
		t.Errorf("wrote %d time(s) for a caller that had given up", p.kicks)
	}
}

func TestTheParkedLoopIsBounded(t *testing.T) {
	ob, ot := Budget, Tick
	Budget, Tick = 60*time.Millisecond, 5*time.Millisecond
	defer func() { Budget, Tick = ob, ot }()

	p := &stallingDouble{}
	_, err := New(engine.NewRunner(), p.eval).Read(context.Background(), "t")
	if err == nil || !strings.Contains(err.Error(), "never settled") {
		t.Fatalf("err = %v, want a settle timeout", err)
	}
}

type stallingDouble struct{}

func (p *stallingDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	*out = ""
	return nil
}
