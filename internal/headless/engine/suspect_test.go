package engine

// Decision A, held by test: the signal stays, and the profile carries the doubt.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The claim the decision rests on: a dirty stop is not merely reported, it is
// RECORDED where the next boot will find it.
func TestCleanStopMarksTheProfileWhenTheStopGoesDirty(t *testing.T) {
	dir := t.TempDir()
	// exits=false forces the wait to time out, which is what drives the signal.
	p := newFakeProcessInProfile("ws://127.0.0.1:0/devtools/browser/x", false, dir)

	r := shortShutdownRunner()
	via := CleanStop(context.Background(), r, p)

	if via.Clean() {
		t.Fatalf("stopped_via=%s: this case exists to produce a DIRTY stop", via)
	}
	suspect, recorded, err := SessionSuspect(dir)
	if err != nil {
		t.Fatalf("SessionSuspect: %v", err)
	}
	if !suspect {
		t.Fatal("the browser went down by signal and the profile was left unmarked; " +
			"the next boot would presume the session survived, which is exactly " +
			"what decision A buys with the signal it keeps")
	}
	if recorded != via {
		t.Errorf("marker records %q, CleanStop returned %q; a mark that names the "+
			"wrong cause is worse than none", recorded, via)
	}
}

// The other half, and the one that decides whether the mark means anything: a
// CLEAN stop must leave nothing behind. A marker written on every stop would be
// a permanent alarm, and a permanent alarm is ignored.
func TestCleanStopLeavesNoMarkWhenTheStopIsClean(t *testing.T) {
	dir := t.TempDir()
	p := newFakeProcessInProfile("ws://127.0.0.1:0/devtools/browser/x", true, dir)

	via := CleanStop(context.Background(), shortShutdownRunner(), p)

	if !via.Clean() {
		t.Fatalf("stopped_via=%s: this case exists to produce a CLEAN stop", via)
	}
	suspect, _, err := SessionSuspect(dir)
	if err != nil {
		t.Fatalf("SessionSuspect: %v", err)
	}
	if suspect {
		t.Fatal("a clean stop marked the session suspect; the signal would then be " +
			"indistinguishable from the protocol path, which is the trap that made " +
			"the study's browserclose arm a replica of its own control")
	}
}

// The doubt has to outlive a restart, or it protects only the boot that happens
// to read it first.
func TestSuspectMarkSurvivesReadingAndIsClearedOnlyExplicitly(t *testing.T) {
	dir := t.TempDir()
	if err := MarkSessionSuspect(dir, StopViaDirtySignalCloseRefused); err != nil {
		t.Fatalf("MarkSessionSuspect: %v", err)
	}

	for i := range 3 {
		suspect, _, err := SessionSuspect(dir)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if !suspect {
			t.Fatalf("read %d cleared the doubt; a boot that read the state and then "+
				"crashed would lose it", i)
		}
	}

	if err := ClearSessionSuspect(dir); err != nil {
		t.Fatalf("ClearSessionSuspect: %v", err)
	}
	if suspect, _, _ := SessionSuspect(dir); suspect {
		t.Fatal("the mark survived an explicit clear")
	}
	// Clearing twice is not an error: verification may legitimately run again.
	if err := ClearSessionSuspect(dir); err != nil {
		t.Errorf("clearing an already-clear profile errored: %v", err)
	}
}

// An unreadable profile is NOT a healthy profile. Folding the error into false
// would turn "we could not check" into "it is fine", which is the fail-open
// shape invariant 12 forbids.
func TestSessionSuspectRefusesToGuessOnAnUnreadableMarker(t *testing.T) {
	dir := t.TempDir()
	// A directory where the marker file should be: reading it yields an error
	// that is not ErrNotExist.
	if err := os.Mkdir(filepath.Join(dir, suspectMarker), 0o700); err != nil {
		t.Fatalf("staging: %v", err)
	}

	suspect, _, err := SessionSuspect(dir)
	if err == nil {
		t.Fatal("an unreadable marker was reported as an answer; a profile we cannot " +
			"read must not be declared healthy")
	}
	if suspect {
		t.Error("suspect=true was returned alongside an error; the caller should get " +
			"the error, not a guess in either direction")
	}
}

// A browser with no profile holds no session, so there is nothing to doubt.
func TestMarkingIsANoopWithoutAProfile(t *testing.T) {
	if err := MarkSessionSuspect("", StopViaDirtySignalExitTimeout); err != nil {
		t.Errorf("marking a profile-less browser errored: %v", err)
	}
	if suspect, _, err := SessionSuspect(""); suspect || err != nil {
		t.Errorf("SessionSuspect(\"\") = %v, %v; want false, nil", suspect, err)
	}
}
