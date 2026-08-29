package contacts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/headless/engine"
)

// A LABEL ID THIS ACCOUNT DOES NOT HAVE IS ITS OWN ERROR, not a zero Label.
//
// The ids on this build are short strings ("1", "2", "3"), where a typo is not
// something anyone notices by reading — "1" against "l" looks the same. A caller
// that got an empty Label back would ship that typo.
func TestALabelIdThatDoesNotExistIsItsOwnError(t *testing.T) {
	l := New(engine.NewRunner(), labelEval(`{"ok":true,"why":"","rows":[
	 {"id":"1","name":"a","color":0,"count":2},
	 {"id":"2","name":"b","color":1,"count":0}]}`))
	_, err := l.LabelByID(context.Background(), "9", "t")
	if !errors.Is(err, ErrNoLabel) {
		t.Fatalf("err = %v, want ErrNoLabel", err)
	}
	if !strings.Contains(err.Error(), "known") {
		t.Errorf("the error does not say how many were searched: %v", err)
	}
}

func TestALabelIdThatExistsComesBackWhole(t *testing.T) {
	l := New(engine.NewRunner(), labelEval(`{"ok":true,"why":"","rows":[
	 {"id":"1","name":"a","color":0,"count":0},
	 {"id":"2","name":"b","color":1,"count":7}]}`))
	got, err := l.LabelByID(context.Background(), "2", "t")
	if err != nil {
		t.Fatalf("LabelByID: %v", err)
	}
	if got.ID != "2" || got.Count != 7 {
		t.Fatalf("the wrong label came back: %+v", got)
	}
	// A LABEL WITH ZERO THINGS IS STILL A LABEL. Measured: this account's three
	// labels all carry zero items (H114), so a lookup that treated count 0 as
	// absent would find none of them.
	zero, err := l.LabelByID(context.Background(), "1", "t")
	if err != nil {
		t.Fatalf("LabelByID(count 0): %v", err)
	}
	if zero.ID != "1" {
		t.Error("a label with a zero count was treated as absent")
	}
}

func TestAnEmptyLabelIdNeverReachesThePage(t *testing.T) {
	asked := false
	l := New(engine.NewRunner(), func(ctx context.Context, expr string, out *string) error {
		asked = true
		*out = `{"ok":true,"why":"","rows":[]}`
		return nil
	})
	if _, err := l.LabelByID(context.Background(), " ", "t"); !errors.Is(err, ErrNoLabel) {
		t.Fatalf("err = %v, want ErrNoLabel", err)
	}
	if asked {
		t.Error("an empty label id reached the page")
	}
}
