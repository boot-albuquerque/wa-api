package addressbook

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wa-api/internal/wa-headless/engine"
)

type double struct {
	saveAnswer string
	nameAnswer string
	// namedAfter makes the name-state read answer "not named" for the first N
	// calls, which is what the page does while the record catches up.
	namedAfter int

	nameReads  int
	kicks      int
	reads      int
	lastScript string
}

func (d *double) eval(ctx context.Context, expr string, out *string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(expr, "window."+stateKey):
		d.reads++
		*out = d.saveAnswer
		return nil
	case strings.Contains(expr, "named: named(find("):
		d.nameReads++
		if d.nameReads <= d.namedAfter {
			*out = `{"ok":true,"named":false}`
			return nil
		}
		if d.nameAnswer != "" {
			*out = d.nameAnswer
			return nil
		}
		*out = `{"ok":true,"named":true}`
		return nil
	default:
		d.kicks++
		d.lastScript = expr
		*out = "kicked"
		return nil
	}
}

func mgr(d *double) *Manager { return New(engine.NewRunner(), d.eval) }

func fast(t *testing.T) {
	t.Helper()
	oldB, oldT := Budget, Tick
	Budget, Tick = 300*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { Budget, Tick = oldB, oldT })
}

// THE JID A CALLER ALREADY HAS IS THE JID IT WILL PASS.
//
// The page wants digits — the reference documents "17182222222" — and every jid
// in this module arrives as "17182222222@c.us". A save that forwarded the whole
// jid would store a contact under a number that does not exist, and the page
// would report success.
func TestTheNumberIsNormalizedBeforeItReachesThePage(t *testing.T) {
	for _, in := range []string{
		"5541999999999@lid", "5541999999999@lid", "+55 41 99999-9999", "5541999999999",
	} {
		if got := normalizeNumber(in); got != "5541999999999" {
			t.Errorf("normalizeNumber(%q) = %q", in, got)
		}
	}
	fast(t)
	d := &double{saveAnswer: `{"ok":true,"had":false,"has":true}`}
	if _, err := mgr(d).Save(context.Background(), "5541999999999@lid", "Lab", "", false, "t"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// MATCH THE ARGUMENT, NOT THE WORD. The first version of this assertion
	// looked for `@c.us"` anywhere in the script and fired on the prelude's
	// OWN lookup helper, which legitimately builds `pn + "@c.us"` to find the
	// record. Same trap this repository has now hit five times: assert on the
	// call as sent, with its key and its colon.
	if strings.Contains(d.lastScript, `phoneNumber: "5541999999999@`) {
		t.Error("the jid suffix reached the page inside the phoneNumber argument")
	}
	if !strings.Contains(d.lastScript, `phoneNumber: "5541999999999"`) {
		t.Error("the page did not receive the bare digits")
	}
}

// A save with no name at all is refused before it reaches the page. The page
// would accept it and store a nameless contact, which is indistinguishable from
// not having saved.
func TestASaveWithNoNameIsRefused(t *testing.T) {
	d := &double{}
	_, err := mgr(d).Save(context.Background(), "5541999999999", "  ", "", false, "t")
	if !errors.Is(err, ErrNoName) {
		t.Fatalf("err = %v, want ErrNoName", err)
	}
	if d.kicks != 0 {
		t.Error("a nameless save reached the page")
	}
}

// An empty number is refused on every method.
func TestAnEmptyNumberIsRefusedEverywhere(t *testing.T) {
	d := &double{}
	m := mgr(d)
	if _, err := m.Save(context.Background(), "@c.us", "Lab", "", false, "t"); !errors.Is(err, ErrNoNumber) {
		t.Errorf("Save = %v", err)
	}
	if err := m.Delete(context.Background(), "   ", "t"); !errors.Is(err, ErrNoNumber) {
		t.Errorf("Delete = %v", err)
	}
	if _, err := m.DeviceCount(context.Background(), "", "t"); !errors.Is(err, ErrNoNumber) {
		t.Errorf("DeviceCount = %v", err)
	}
	if d.kicks != 0 {
		t.Error("an empty number reached the page")
	}
}

// SYNCING TO THE PHONE IS OPT-IN AND TRAVELS AS ASKED.
//
// True writes the contact into the address book of a physical phone — a change
// outside this process that no test can undo. A default that leaked true would
// be the worst kind of defect: invisible here, permanent there.
func TestTheSyncFlagIsSentExactlyAsGiven(t *testing.T) {
	fast(t)
	for _, want := range []bool{false, true} {
		d := &double{saveAnswer: `{"ok":true,"had":false,"has":true}`}
		got, err := mgr(d).Save(context.Background(), "5541999999999", "Lab", "", want, "t")
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		if got.SyncedToPhone != want {
			t.Errorf("result says syncedToPhone=%t, want %t", got.SyncedToPhone, want)
		}
		marker := "syncToAddressbook: false"
		if want {
			marker = "syncToAddressbook: true"
		}
		if !strings.Contains(d.lastScript, marker) {
			t.Errorf("the page did not receive %q", marker)
		}
	}
}

// A SAVE THAT THE PAGE ACCEPTS AND DOES NOT APPLY IS NOT A SUCCESS.
//
// This module has twice reported a write as done because the call returned
// (H82, H85). The record moves a moment later or it never does, and the second
// case has its own error so that a caller can tell a refusal from a no-op.
func TestAnAcceptedSaveThatNeverLandsIsNotSuccess(t *testing.T) {
	fast(t)
	d := &double{
		saveAnswer: `{"ok":true,"had":false,"has":false}`,
		namedAfter: 1 << 30, // the record never moves
	}
	got, err := mgr(d).Save(context.Background(), "5541999999999", "Lab", "", false, "t")
	if !errors.Is(err, ErrNotSaved) {
		t.Fatalf("err = %v, want ErrNotSaved", err)
	}
	if got.HasName {
		t.Error("the result claims a name")
	}
	if d.nameReads == 0 {
		t.Error("the settle loop never asked the page again")
	}
}

// THE WAITING HAPPENS IN GO. The page parks as soon as it accepts, and the
// polling is here — invariant 6. A script that slept would pass every other
// test in this file and would take its deadline decisions where a caller's
// context cannot reach.
func TestThePageDoesNotWait(t *testing.T) {
	fast(t)
	d := &double{saveAnswer: `{"ok":true,"had":false,"has":false}`, namedAfter: 2}
	got, err := mgr(d).Save(context.Background(), "5541999999999", "Lab", "", false, "t")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !got.HasName {
		t.Fatal("the settle loop did not notice the record moving")
	}
	if d.nameReads < 3 {
		t.Errorf("%d name reads; Go must have polled", d.nameReads)
	}
	for _, banned := range []string{"setTimeout(", "setInterval(", "Date.now("} {
		if strings.Contains(d.lastScript, banned) {
			t.Errorf("the save script contains %q; the clock belongs on this side", banned)
		}
	}
}

// "NO RECORD" AND "ZERO DEVICES" ARE DIFFERENT ANSWERS. Folding them into 0
// would let a user this account has never exchanged keys with look like a user
// with no phone.
func TestAnUnknownUserIsNotZeroDevices(t *testing.T) {
	d := &double{saveAnswer: `{"ok":true,"known":false,"count":0}`}
	_, err := mgr(d).DeviceCount(context.Background(), "5541999999999@lid", "t")
	if !errors.Is(err, ErrDevices) {
		t.Fatalf("err = %v, want ErrDevices", err)
	}
	if !strings.Contains(err.Error(), "no device record") {
		t.Errorf("the reason is unhelpful: %v", err)
	}

	d2 := &double{saveAnswer: `{"ok":true,"known":true,"count":3}`}
	n, err := mgr(d2).DeviceCount(context.Background(), "5541999999999@lid", "t")
	if err != nil || n != 3 {
		t.Fatalf("DeviceCount = %d, %v", n, err)
	}
}

// THE DELETE SENDS A WID, THE SAVE SENDS BARE DIGITS.
//
// Nobody would guess that asymmetry, and the reference hides it by passing the
// same value to both. createWid on bare digits fails on this build with
// "wid error: invalid wid", which is how the first live delete died — after the
// save had already succeeded, leaving a contact behind that the cleanup could
// not remove.
func TestTheDeleteBuildsAWidAndTheSaveDoesNot(t *testing.T) {
	d := &double{saveAnswer: `{"ok":true}`}
	if err := mgr(d).Delete(context.Background(), "5541999999999@lid", "t"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !strings.Contains(d.lastScript, `createWid("5541999999999" + "@c.us")`) {
		t.Error("the delete does not build a wid with the suffix; the page answers 'invalid wid'")
	}

	fast(t)
	d2 := &double{saveAnswer: `{"ok":true,"had":false,"has":true}`}
	if _, err := mgr(d2).Save(context.Background(), "5541999999999@lid", "Lab", "", false, "t"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.Contains(d2.lastScript, `phoneNumber: "5541999999999"`) {
		t.Error("the save does not send bare digits")
	}
}

// A page failure keeps its reason on every method.
func TestPageFailuresKeepTheirReason(t *testing.T) {
	fast(t)
	d := &double{saveAnswer: `{"ok":false,"why":"TypeError message=nope keys=[a]"}`}
	if _, err := mgr(d).Save(context.Background(), "5541999999999", "Lab", "", false, "t"); !errors.Is(err, ErrSave) ||
		!strings.Contains(err.Error(), "nope") {
		t.Errorf("save err = %v", err)
	}
	d2 := &double{saveAnswer: `{"ok":false,"why":"BAD"}`}
	if err := mgr(d2).Delete(context.Background(), "5541999999999", "t"); !errors.Is(err, ErrDelete) ||
		!strings.Contains(err.Error(), "BAD") {
		t.Errorf("delete err = %v", err)
	}
}

// The result renders nothing anybody could dial.
func TestTheResultRendersNoIdentity(t *testing.T) {
	s := Saved{HadName: false, HasName: true, SyncedToPhone: true}
	out := s.String()
	if strings.Contains(out, "5541") || strings.Contains(out, "@") {
		t.Errorf("Saved.String carries identity: %s", out)
	}
}

// THE REFUSAL REPLACES A WELL-FORMED WRONG ANSWER (decisão 66).
//
// Measured side by side on the same peer: under the lid, 5 devices; under the
// phone jid, "the page has no device record for that user". Both are well
// formed, which is what made the error invisible for months — the second reads
// as a fact about the PERSON and is a fact about the ARGUMENT.
func TestDeviceCountRefusesAPhoneJIDDistinctly(t *testing.T) {
	d := &double{saveAnswer: `{"ok":true,"known":true,"count":5}`}
	_, err := mgr(d).DeviceCount(context.Background(), "5541999999999@c.us", "t")
	if !errors.Is(err, ErrUnresolvedIdentity) {
		t.Fatalf("want ErrUnresolvedIdentity, got %v", err)
	}
	if errors.Is(err, ErrDevices) {
		t.Fatal("the refusal is indistinguishable from 'no device record', which " +
			"is the exact confusion measured in H148")
	}
}

// AND IT REFUSES WITHOUT ASKING THE PAGE — no hidden resolution, which is the
// half of decision 66 that rejected option (a).
func TestDeviceCountRefusesWithoutTouchingThePage(t *testing.T) {
	d := &double{saveAnswer: `{"ok":true,"known":true,"count":5}`}
	if _, err := mgr(d).DeviceCount(context.Background(), "5541999999999@c.us", "t"); err == nil {
		t.Fatal("expected a refusal")
	}
	if d.kicks != 0 {
		t.Fatalf("the page was asked %d time(s); a reader must not resolve on its "+
			"own, and a caller in a loop would pay for it without seeing it", d.kicks)
	}
}
