package accounttype

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

// pageDouble answers with raw JSON, matching what spa.Evaluator hands back in
// production — ARMADILHAS §1 requires the double to imitate the REAL shape,
// not a convenient Go struct.
type pageDouble struct {
	answer string
	err    error
	calls  int
}

func (p *pageDouble) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.calls++
	if p.err != nil {
		return p.err
	}
	*out = p.answer
	return nil
}

func detect(t *testing.T, p *pageDouble) (Kind, error) {
	t.Helper()
	return Detect(context.Background(), engine.NewRunner(), p.eval, "test/accounttype")
}

// TestDetect_Business: canSetMyPushname false is the measured signal for a
// Business account (HOUSEKEEP.md, profile.go).
func TestDetect_Business(t *testing.T) {
	kind, err := detect(t, &pageDouble{answer: `{"ok":true,"can_set_my_pushname":false}`})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if kind != KindBusiness {
		t.Errorf("kind = %v, want KindBusiness", kind)
	}
}

// TestDetect_Personal: canSetMyPushname true is a personal account.
func TestDetect_Personal(t *testing.T) {
	kind, err := detect(t, &pageDouble{answer: `{"ok":true,"can_set_my_pushname":true}`})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if kind != KindPersonal {
		t.Errorf("kind = %v, want KindPersonal", kind)
	}
}

// TestDetect_ModuleAbsent: ok=false (getter missing in this build) must never
// become a business/personal guess.
func TestDetect_ModuleAbsent(t *testing.T) {
	kind, err := detect(t, &pageDouble{answer: `{"ok":false,"can_set_my_pushname":false}`})
	if err == nil {
		t.Fatal("expected error when the getter did not resolve")
	}
	if kind != KindUnknown {
		t.Errorf("kind = %v, want KindUnknown", kind)
	}
}

// TestDetect_EvalError: a transport/tab failure must not become a silent
// KindUnknown-with-nil-error — the caller needs to see the probe failed.
func TestDetect_EvalError(t *testing.T) {
	wantErr := errors.New("tab is gone")
	kind, err := detect(t, &pageDouble{err: wantErr})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want wrapping %v", err, wantErr)
	}
	if kind != KindUnknown {
		t.Errorf("kind = %v, want KindUnknown", kind)
	}
}

// TestDetect_UnexpectedShape: an answer this build cannot parse must not be
// mistaken for either classification.
func TestDetect_UnexpectedShape(t *testing.T) {
	kind, err := detect(t, &pageDouble{answer: `not json at all`})
	if err == nil {
		t.Fatal("expected error for unparsable answer")
	}
	if kind != KindUnknown {
		t.Errorf("kind = %v, want KindUnknown", kind)
	}
}
