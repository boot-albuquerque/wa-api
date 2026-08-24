package spa

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

type versionDouble struct {
	answer string
	err    error
	asked  string
}

func (d *versionDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	d.asked = expr
	if d.err != nil {
		return d.err
	}
	*out = d.answer
	return nil
}

func TestTheWebVersionIsReadFromThePage(t *testing.T) {
	d := &versionDouble{answer: `{"ok":true,"version":"2.3000.1234567890"}`}
	got, err := WebVersion(context.Background(), engine.NewRunner(), d.eval)
	if err != nil {
		t.Fatalf("WebVersion: %v", err)
	}
	if got != "2.3000.1234567890" {
		t.Fatalf("version = %q", got)
	}
	if !strings.Contains(d.asked, "window.Debug") {
		t.Error("the script does not read window.Debug.VERSION")
	}
}

// A PAGE THAT HIDES IT IS NOT AN EMPTY STRING. A caller that got "" could not
// tell "this build hides it" from "the read failed", and the repairs differ.
func TestAPageWithoutAVersionIsItsOwnError(t *testing.T) {
	for _, answer := range []string{
		`{"ok":true,"version":""}`,
		`{"ok":true,"version":"   "}`,
		`{"ok":false}`,
	} {
		d := &versionDouble{answer: answer}
		_, err := WebVersion(context.Background(), engine.NewRunner(), d.eval)
		if !errors.Is(err, ErrNoWebVersion) {
			t.Errorf("answer %s gave err = %v, want ErrNoWebVersion", answer, err)
		}
	}
}

func TestAnUnexpectedShapeIsRefusedRatherThanGuessed(t *testing.T) {
	d := &versionDouble{answer: `not json`}
	_, err := WebVersion(context.Background(), engine.NewRunner(), d.eval)
	if err == nil {
		t.Fatal("a non-JSON answer was accepted")
	}
	if errors.Is(err, ErrNoWebVersion) {
		t.Error("a broken answer is being reported as a page that hides the version")
	}
}

func TestACancelledCallerNeverAsksForTheVersion(t *testing.T) {
	d := &versionDouble{answer: `{"ok":true,"version":"x"}`}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := WebVersion(ctx, engine.NewRunner(), d.eval); err == nil {
		t.Fatal("a cancelled context produced a version")
	}
	if d.asked != "" {
		t.Error("the page was asked for a caller that had given up")
	}
}
