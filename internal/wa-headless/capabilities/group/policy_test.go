package group

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"wa-api/internal/wa-headless/engine"
)

type policyDouble struct {
	ok         bool
	stage, why string
	already    bool

	kicks      int
	lastScript string
}

func (p *policyDouble) eval(ctx context.Context, expr string, out *string) error {
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
		*out = fmt.Sprintf(`{"stage":"done","ok":true,"why":"","already":%t}`, p.already)
		return nil
	}
	p.kicks++
	p.lastScript = expr
	*out = `{"started":true}`
	return nil
}

func policer(p *policyDouble) *Manager { return New(engine.NewRunner(), p.eval) }

// TestOnlyTheEnumeratedNamesAreSent, and the obvious guesses are refused.
//
// `locked` and `announce` read like the right words for "only admins may edit"
// and "only admins may send". Both were measured REFUSED by the page. The names
// that work are `restrict` and `announcement`, and nothing but enumeration would
// have produced that pairing.
func TestOnlyTheEnumeratedNamesAreSent(t *testing.T) {
	compressPartClock(t)
	for _, p := range []Policy{PolicyMessagesAdminsOnly, PolicyInfoAdminsOnly, PolicyJoinNeedsApproval} {
		d := &policyDouble{ok: true}
		if _, err := policer(d).SetPolicy(context.Background(), testGroupJID, p, true, "t"); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if !strings.Contains(d.lastScript, `setGroupProperty(chat, "`+string(p)+`", 1)`) {
			t.Fatalf("%s does not reach setGroupProperty with its measured name", p)
		}
	}
	// The words that look right and are not.
	for _, bad := range []Policy{"locked", "announce", "description", "subject"} {
		d := &policyDouble{ok: true}
		_, err := policer(d).SetPolicy(context.Background(), testGroupJID, bad, true, "t")
		if !errors.Is(err, ErrUnknownPolicy) {
			t.Fatalf("%q: got %v, want ErrUnknownPolicy", bad, err)
		}
		if d.kicks != 0 {
			t.Fatalf("%q reached the page", bad)
		}
	}
}

// TestTheValueIsOneOrZero. The app's switch compares the value to 1; a boolean
// would compare false and silently mean "off" for every call.
func TestTheValueIsOneOrZero(t *testing.T) {
	compressPartClock(t)
	on := &policyDouble{ok: true}
	if _, err := policer(on).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t"); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if !strings.Contains(on.lastScript, `"restrict", 1)`) {
		t.Fatal("on did not become 1")
	}
	off := &policyDouble{ok: true}
	if _, err := policer(off).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, false, "t"); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if !strings.Contains(off.lastScript, `"restrict", 0)`) {
		t.Fatal("off did not become 0")
	}
	if strings.Contains(on.lastScript, `"restrict", true)`) {
		t.Fatal("a boolean is being sent where the app compares to 1")
	}
}

// TestTheNameSetAndTheFieldREADAreDifferentStrings is the asymmetry this
// codebase keeps producing: `announcement` is written and `announce` is read.
func TestTheNameSetAndTheFieldReadAreDifferentStrings(t *testing.T) {
	if field[PolicyMessagesAdminsOnly] == string(PolicyMessagesAdminsOnly) {
		t.Fatal("the read field equals the write name; measured, they differ")
	}
	if !strings.Contains(policyReadScript(testGroupJID, PolicyMessagesAdminsOnly), `"announce"`) {
		t.Fatal("the read does not use the metadata's own field name")
	}
	if !strings.Contains(policyScript(testGroupJID, PolicyMessagesAdminsOnly, true), `"announcement"`) {
		t.Fatal("the write does not use the property name")
	}
}

// TestTheGroupsOwnGateIsAskedFirst — canSetGroupProperty is a METHOD, the
// lesson H57 paid four blind attempts for.
func TestTheGroupsOwnGateIsAskedFirst(t *testing.T) {
	compressPartClock(t)
	d := &policyDouble{ok: false, stage: "find", why: "CANNOT_SET"}
	if _, err := policer(d).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t"); !errors.Is(err, ErrCannotSetPolicy) {
		t.Fatalf("got %v, want ErrCannotSetPolicy", err)
	}
	if !strings.Contains(d.lastScript, "typeof md.canSetGroupProperty === 'function' && !md.canSetGroupProperty()") {
		t.Fatal("the gate is not asked as a METHOD, or does not guard a return")
	}
}

// TestARealPolicyChangeIsUnverified, for the reason H58 measured.
func TestARealPolicyChangeIsUnverified(t *testing.T) {
	compressPartClock(t)
	d := &policyDouble{ok: true}
	got, err := policer(d).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t")
	if err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if got.Verified || got.NoOp {
		t.Fatalf("a real change claims to be confirmed: %s", got)
	}
	if strings.Contains(d.lastScript, "stage: 'settling'") {
		t.Fatal("the script waits on metadata this build does not refresh in-session")
	}
	noop := &policyDouble{ok: true, already: true}
	n, err := policer(noop).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t")
	if err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if !n.NoOp || !n.Verified {
		t.Fatalf("a no-op is not reported as confirmed: %s", n)
	}
}

func TestPolicyRefusalsCostNoPageCall(t *testing.T) {
	compressPartClock(t)
	d := &policyDouble{ok: true}
	if _, err := policer(d).SetPolicy(context.Background(), "1@c.us", PolicyInfoAdminsOnly, true, "t"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup", err)
	}
	if _, err := policer(d).PolicyOf(context.Background(), "1@c.us", PolicyInfoAdminsOnly, "t"); !errors.Is(err, ErrNotGroup) {
		t.Fatalf("got %v, want ErrNotGroup", err)
	}
	if _, err := policer(d).PolicyOf(context.Background(), testGroupJID, "locked", "t"); !errors.Is(err, ErrUnknownPolicy) {
		t.Fatalf("got %v, want ErrUnknownPolicy", err)
	}
	if d.kicks != 0 {
		t.Fatalf("the page was asked %d time(s)", d.kicks)
	}
}

func TestPolicyOfReadsBothAnswers(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
		bad  bool
	}{{"true", true, false}, {"false", false, false}, {"no", false, true}} {
		m := New(engine.NewRunner(), func(_ context.Context, _ string, out *string) error {
			*out = tc.raw
			return nil
		})
		got, err := m.PolicyOf(context.Background(), testGroupJID, PolicyInfoAdminsOnly, "t")
		if tc.bad {
			if err == nil {
				t.Fatalf("%q was accepted as an answer", tc.raw)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%q: got (%t, %v), want (%t, nil)", tc.raw, got, err, tc.want)
		}
	}
}

func TestCancelledContextChangesNoPolicy(t *testing.T) {
	compressPartClock(t)
	d := &policyDouble{ok: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := policer(d).SetPolicy(ctx, testGroupJID, PolicyInfoAdminsOnly, true, "t"); err == nil {
		t.Fatal("a cancelled context changed a group policy")
	}
	if d.kicks != 0 {
		t.Fatalf("asked the page %d time(s) for a caller that had given up", d.kicks)
	}
}

func TestPolicyFailuresNameTheirCause(t *testing.T) {
	compressPartClock(t)
	for _, tc := range []struct {
		why  string
		want error
	}{
		{"NO_CHAT", ErrNotGroup},
		{"NO_METADATA", ErrNotGroup},
	} {
		d := &policyDouble{ok: false, stage: "find", why: tc.why}
		if _, err := policer(d).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t"); !errors.Is(err, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.why, err, tc.want)
		}
	}
	threw := &policyDouble{ok: false, stage: "apply", why: "boom"}
	_, err := policer(threw).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t")
	if !errors.Is(err, ErrPolicy) || !strings.Contains(err.Error(), "at apply (boom)") {
		t.Fatalf("the failure does not name its stage: %v", err)
	}
}

func TestAHungPageIsATimeoutForPolicies(t *testing.T) {
	compressPartClock(t)
	stuck := func(_ context.Context, expr string, out *string) error {
		if strings.Contains(expr, "const s = window[") {
			*out = `{"stage":"pending","ok":false,"why":""}`
			return nil
		}
		*out = `{"started":true}`
		return nil
	}
	m := New(engine.NewRunner(), stuck)
	if _, err := m.SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t"); !errors.Is(err, ErrPolicy) {
		t.Fatalf("got %v, want ErrPolicy", err)
	}
}

func TestPolicyEvaluatorErrorsSurface(t *testing.T) {
	compressPartClock(t)
	boom := func(context.Context, string, *string) error { return errors.New("evaluator down") }
	m := New(engine.NewRunner(), boom)
	if _, err := m.SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t"); !errors.Is(err, ErrPolicy) {
		t.Fatalf("got %v, want ErrPolicy", err)
	}
	if _, err := m.PolicyOf(context.Background(), testGroupJID, PolicyInfoAdminsOnly, "t"); !errors.Is(err, ErrPolicy) {
		t.Fatalf("got %v, want ErrPolicy", err)
	}
	bad := func(_ context.Context, _ string, out *string) error {
		*out = "not json"
		return nil
	}
	if _, err := New(engine.NewRunner(), bad).SetPolicy(context.Background(), testGroupJID, PolicyInfoAdminsOnly, true, "t"); err == nil {
		t.Fatal("a non-JSON answer was accepted")
	}
}

// TestEveryPolicyHasAReadField. A policy with no entry in the field map reads as
// false forever, which would make its live proof pass by accident.
func TestEveryPolicyHasAReadField(t *testing.T) {
	for p := range known {
		f, ok := field[p]
		if !ok || f == "" {
			t.Errorf("policy %q has no metadata field to read back", p)
		}
	}
	if len(field) != len(known) {
		t.Errorf("the field map has %d entries and the known set has %d; one of them "+
			"was extended without the other", len(field), len(known))
	}
}
