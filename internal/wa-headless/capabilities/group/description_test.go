package group

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// describeDouble answers the description write AND the verifying metadata read,
// keeping the server's text as its OWN state — which is the only way a test can
// make the page accept the call and the server keep the old value. That is the
// failure the reference cannot report at all: it returns a boolean computed from
// the absence of an exception.
type describeDouble struct {
	serverText string
	// takes says whether the page actually stores what it was given.
	takes bool
	// refuse makes the page reject the write.
	refuse string

	answer  string
	scripts []string
}

func (d *describeDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does.
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(expr, "window."+describeKey) ||
		strings.HasPrefix(expr, "window."+joinStateKey) {
		*out = d.answer
		return nil
	}
	d.scripts = append(d.scripts, expr)
	switch {
	case strings.Contains(expr, "setGroupDescription"):
		if d.refuse != "" {
			d.answer = `{"ok":false,"why":"` + d.refuse + `"}`
			break
		}
		if d.takes {
			d.serverText = between(expr, `description: "`, `"`)
		}
		d.answer = `{"ok":true}`
	default: // the metadata read
		d.answer = `{"ok":true,"jid":"1@g.us","description":"` + d.serverText +
			`","descriptionSource":"displayedDesc","participants":[]}`
	}
	*out = "kicked"
	return nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	if j := strings.Index(s, b); j >= 0 {
		return s[:j]
	}
	return s
}

func describer(d *describeDouble) *Manager { return New(engine.NewRunner(), d.eval) }

func fastDescribe(t *testing.T) {
	t.Helper()
	ob, ot := describeBudget, describeTick
	describeBudget, describeTick = 200*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { describeBudget, describeTick = ob, ot })
}

// A DESCRIPTION THE SERVER IGNORED IS NOT A SUCCESS.
func TestADescriptionTheServerIgnoredIsAnError(t *testing.T) {
	fastDescribe(t)
	d := &describeDouble{serverText: "old", takes: false}
	_, err := describer(d).SetDescription(context.Background(), "1@g.us", "new", "t")
	if !errors.Is(err, ErrDescriptionNotTaken) {
		t.Fatalf("err = %v, want ErrDescriptionNotTaken", err)
	}
	if !strings.Contains(err.Error(), "server reports") {
		t.Errorf("the error does not say what the server holds: %v", err)
	}
}

// A NON-GROUP NEVER REACHES THE PAGE.
func TestANonGroupIsRefusedBeforeDescribing(t *testing.T) {
	fastDescribe(t)
	d := &describeDouble{}
	if _, err := describer(d).SetDescription(context.Background(), "1@c.us", "x", "t"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("err = %v, want ErrNotGroup", err)
	}
	if len(d.scripts) != 0 {
		t.Error("a one-to-one jid reached the page")
	}
}

// THE SCRIPT MUST PASS FOUR ARGUMENTS, and the fourth must come from the
// group's metadata rather than be invented. The double supplies the outcome, so
// only the script can be asked about this.
func TestTheScriptPassesTheReplacedDescriptionId(t *testing.T) {
	fastDescribe(t)
	d := &describeDouble{takes: true}
	_, _ = describer(d).SetDescription(context.Background(), "1@g.us", "x", "t")
	var script string
	for _, s := range d.scripts {
		if strings.Contains(s, "setGroupDescription") {
			script = s
		}
	}
	if script == "" {
		t.Fatal("no description script was run")
	}
	// THE CALL TAKES ONE OBJECT, not four positional arguments. The reference
	// passes four; this build's function has arity 1, and the positional form
	// dies with "Cannot read properties of undefined (reading 'toJid')" — an
	// error that says nothing about arity and cost a live run to decode (H126).
	if !strings.Contains(script, "groupWid: wid") {
		t.Error("the call does not pass a groupWid field; this build takes one object")
	}
	if !strings.Contains(script, "prevDescId: descId") {
		t.Error("the call does not pass the replaced description id")
	}
	if strings.Contains(script, "wid, \"") {
		t.Error("the call still uses the reference's positional form")
	}
	if !strings.Contains(script, "md.descId || md.__x_descId") {
		t.Error("descId is not read from the group's own metadata under both " +
			"spellings; this build keeps group state under __x_ on live models")
	}
	if !strings.Contains(script, "newId()") {
		t.Error("the script does not mint a new message key")
	}
}

// AN EMPTY DESCRIPTION IS ALLOWED: clearing one is legitimate.
func TestAnEmptyDescriptionIsAllowed(t *testing.T) {
	fastDescribe(t)
	d := &describeDouble{serverText: "something", takes: true}
	if _, err := describer(d).SetDescription(context.Background(), "1@g.us", "", "t"); err != nil {
		t.Fatalf("clearing a description was refused: %v", err)
	}
}

// AN OVERLONG DESCRIPTION IS REFUSED HERE, not by the server.
func TestAnOverlongDescriptionNeverReachesThePage(t *testing.T) {
	fastDescribe(t)
	d := &describeDouble{}
	long := strings.Repeat("a", MaxDescriptionBytes+1)
	if _, err := describer(d).SetDescription(context.Background(), "1@g.us", long, "t"); err == nil {
		t.Fatal("an overlong description was accepted")
	}
	if len(d.scripts) != 0 {
		t.Error("an overlong description reached the page")
	}
}

func TestAPageRefusalIsItsOwnError(t *testing.T) {
	fastDescribe(t)
	d := &describeDouble{refuse: "boom"}
	_, err := describer(d).SetDescription(context.Background(), "1@g.us", "x", "t")
	if !errors.Is(err, ErrDescribe) {
		t.Fatalf("err = %v, want ErrDescribe", err)
	}
	if errors.Is(err, ErrDescriptionNotTaken) {
		t.Error("a refusal is being reported as a change that did not take")
	}
}

func TestACancelledCallerNeverDescribes(t *testing.T) {
	fastDescribe(t)
	d := &describeDouble{takes: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := describer(d).SetDescription(ctx, "1@g.us", "x", "t"); err == nil {
		t.Fatal("a cancelled context changed a description")
	}
	if len(d.scripts) != 0 {
		t.Error("the page was written for a caller that had given up")
	}
}
