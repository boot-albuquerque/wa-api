package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wa-api/internal/headless/engine"
)

// primeDouble reproduces the SHAPE the live page returns, including the parts
// that were surprising: the sync answers `undefined`, so there is nothing to
// read from it, and the roster is measured from the collection instead.
type primeDouble struct {
	before, after wireSnapshot
	failStage     string
	failWhy       string
	kicks         int
	// markBefore/markAfter reproduce the page's refresh mark. They DIFFER by
	// default, because a page whose sync ran is the ordinary case; a test that
	// wants the "it never ran" shape sets them equal, which is exactly the
	// no-op the detector exists to catch.
	markBefore, markAfter string
}

func (p *primeDouble) marks() (string, string) {
	if p.markBefore == "" && p.markAfter == "" {
		return "526473", "549772" // the measured before/after
	}
	return p.markBefore, p.markAfter
}

func (p *primeDouble) eval(ctx context.Context, expr string, out *string) error {
	// THE DOUBLE HONOURS ctx, because the production Evaluate does (H30).
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.Contains(expr, "const s = window[") {
		if p.failStage != "" {
			*out = fmt.Sprintf(`{"stage":%q,"ok":false,"why":%q}`, p.failStage, p.failWhy)
			return nil
		}
		mb, ma := p.marks()
		*out = fmt.Sprintf(
			`{"stage":"done","ok":true,"why":"","before":%s,"after":%s,`+
				`"mark_before":%q,"mark_after":%q}`,
			snapJSON(p.before), snapJSON(p.after), mb, ma)
		return nil
	}
	p.kicks++
	*out = `{"started":true}`
	return nil
}

func snapJSON(s wireSnapshot) string {
	return fmt.Sprintf(`{"total":%d,"with_pushname":%d,"with_name":%d,`+
		`"with_verified_name":%d,"lid_with_phone":%d,"sync_pending":%d}`,
		s.Total, s.WithPushname, s.WithName, s.WithVerifiedName, s.LidWithPhone, s.SyncPending)
}

func primer(p *primeDouble) *Lister { return New(engine.NewRunner(), p.eval) }

func compressPrimeClock(t *testing.T) {
	t.Helper()
	ob, ot := primeBudget, primeTick
	primeBudget, primeTick = 200*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { primeBudget, primeTick = ob, ot })
}

// TestAShrinkingRosterIsAFailure is the postcondition this capability exists
// for, and the only outcome a caller could not detect on their own.
//
// The refresh MUTATES — the sync family writes through
// LidAwareContactsDB.bulkCreateOrMerge — so the failure worth catching is not
// "nothing improved" but "contacts disappeared". A refresh that loses people is
// worse than no refresh at all.
func TestAShrinkingRosterIsAFailure(t *testing.T) {
	compressPrimeClock(t)
	p := &primeDouble{
		before: wireSnapshot{Total: 944, WithPushname: 456, WithName: 1, WithVerifiedName: 52},
		after:  wireSnapshot{Total: 900, WithPushname: 456, WithName: 1, WithVerifiedName: 52},
	}
	_, err := primer(p).Prime(context.Background(), "t/prime")
	if !errors.Is(err, ErrRosterShrank) {
		t.Fatalf("got %v, want ErrRosterShrank: 944 contacts became 900 and the "+
			"capability reported success", err)
	}
	if !strings.Contains(err.Error(), "944") || !strings.Contains(err.Error(), "900") {
		t.Fatalf("the error must carry both counts so the loss is legible: %v", err)
	}
}

// TestNothingChangingIsSUCCESS, because it is the measured ordinary case: the
// roster had no membership gap (391 of 391 chat-referenced contacts already
// present), so a refresh has nothing to add. Reporting that as an error would
// make every healthy run look broken.
func TestNothingChangingIsSuccess(t *testing.T) {
	compressPrimeClock(t)
	same := wireSnapshot{Total: 944, WithPushname: 456, WithName: 1, WithVerifiedName: 52}
	got, err := primer(&primeDouble{before: same, after: same}).Prime(context.Background(), "t/prime")
	if err != nil {
		t.Fatalf("an unchanged roster produced an error: %v", err)
	}
	if got.Changed() {
		t.Fatalf("Changed()=true for identical snapshots: %s", got)
	}
	if got.Added() != 0 {
		t.Fatalf("Added()=%d, want 0", got.Added())
	}
}

// TestTheMeasuredEFFECTIsReported. On the live account the only number that
// moved was verified business names, 52 to 56 — and a result that hid that
// would leave the caller unable to tell a refresh that did something from one
// that did nothing.
func TestTheMeasuredEffectIsReported(t *testing.T) {
	compressPrimeClock(t)
	got, err := primer(&primeDouble{
		before: wireSnapshot{Total: 944, WithPushname: 456, WithName: 1, WithVerifiedName: 52},
		after:  wireSnapshot{Total: 944, WithPushname: 456, WithName: 1, WithVerifiedName: 56},
	}).Prime(context.Background(), "t/prime")
	if err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if !got.Changed() {
		t.Fatalf("Changed()=false while verified names went 52 -> 56: %s", got)
	}
	if got.Added() != 0 {
		t.Fatalf("Added()=%d — no contact appeared, only a name was verified", got.Added())
	}
	if got.Before.WithVerifiedName != 52 || got.After.WithVerifiedName != 56 {
		t.Fatalf("the numbers that moved were not carried: %s", got)
	}
}

// TestAGrowingRosterIsReportedAsAddition guards the other side of the
// postcondition: growth must not be mistaken for damage.
func TestAGrowingRosterIsReportedAsAddition(t *testing.T) {
	compressPrimeClock(t)
	got, err := primer(&primeDouble{
		before: wireSnapshot{Total: 900},
		after:  wireSnapshot{Total: 944},
	}).Prime(context.Background(), "t/prime")
	if err != nil {
		t.Fatalf("a growing roster produced an error: %v", err)
	}
	if got.Added() != 44 {
		t.Fatalf("Added()=%d, want 44", got.Added())
	}
}

func TestAPageThatRefusesIsErrPrime(t *testing.T) {
	compressPrimeClock(t)
	p := &primeDouble{failStage: "sync", failWhy: "boom"}
	_, err := primer(p).Prime(context.Background(), "t/prime")
	if !errors.Is(err, ErrPrime) {
		t.Fatalf("got %v, want ErrPrime", err)
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "sync") {
		t.Fatalf("the stage and reason were dropped: %v", err)
	}
}

// TestAStallIsNotConfusedWithSlowness. The measured cost is 41.9s, so the
// budget is minutes; a timeout at that scale means the page stopped, and the
// message says so rather than inviting someone to raise the number again.
func TestAStallIsNotConfusedWithSlowness(t *testing.T) {
	compressPrimeClock(t)
	p := &primeDouble{}
	// Never settles: the double keeps answering "pending".
	p.failStage = "pending"
	_, err := primer(p).Prime(context.Background(), "t/prime")
	if !errors.Is(err, ErrPrime) {
		t.Fatalf("got %v, want ErrPrime", err)
	}
	if !strings.Contains(err.Error(), "41.9s") {
		t.Fatalf("the message must carry the measured cost so a stall is not read "+
			"as slowness: %v", err)
	}
}

func TestCancelledContextNeverStartsARefresh(t *testing.T) {
	compressPrimeClock(t)
	p := &primeDouble{before: wireSnapshot{Total: 1}, after: wireSnapshot{Total: 1}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := primer(p).Prime(ctx, "t/prime"); err == nil {
		t.Fatal("a cancelled context started a 40-second mutating refresh")
	}
	if p.kicks != 0 {
		t.Fatalf("kicked the sync %d time(s) for a caller that had given up", p.kicks)
	}
}

// TestTheLinkageIsWhatGetsReported is the correction the live proof forced.
//
// The first version of this capability counted names and called a refresh that
// linked 23 more lid rows to their phone numbers "changed=false". It was
// measuring the wrong thing: the roster read 521 people from the same 944 rows
// where it had read 544, because more rows had gained the phone that lets them
// merge. Every new link is one person who stops being counted twice.
func TestTheLinkageIsWhatGetsReported(t *testing.T) {
	compressPrimeClock(t)
	got, err := primer(&primeDouble{
		before: wireSnapshot{Total: 944, WithPushname: 456, WithVerifiedName: 56, LidWithPhone: 398},
		after:  wireSnapshot{Total: 944, WithPushname: 456, WithVerifiedName: 56, LidWithPhone: 421},
	}).Prime(context.Background(), "t/prime")
	if err != nil {
		t.Fatalf("Prime: %v", err)
	}
	if got.Linked() != 23 {
		t.Fatalf("Linked()=%d, want 23", got.Linked())
	}
	if !got.Changed() {
		t.Fatal("Changed()=false while 23 rows gained their phone number: this is " +
			"exactly the omission the live proof exposed")
	}
	if got.Added() != 0 {
		t.Fatalf("Added()=%d — no contact appeared; they became linkable", got.Added())
	}
	if !strings.Contains(got.String(), "linked=23") {
		t.Fatalf("the rendering hides the only number that moved: %s", got)
	}
}

// TestARefreshThatNEVERRANIsAFailure is the postcondition the orchestration
// required, and the gap this capability shipped with until it was named.
//
// The sync returns undefined, so its return value proves nothing. Without a
// mark, "the roster was already current" and "doFullContactSync was never
// called" are the SAME observation — and the earlier version reported both as
// success. A detector that cannot fail is not a detector.
//
// The mark is the page's own contact-sync-refresh-seconds, which a 45-second
// idle control showed does not drift on its own and which every measured sync
// moved.
func TestARefreshThatNeverRanIsAFailure(t *testing.T) {
	compressPrimeClock(t)
	same := wireSnapshot{Total: 944, WithPushname: 456, LidWithPhone: 421, SyncPending: 31}
	p := &primeDouble{
		before:     same,
		after:      same,
		markBefore: "526473",
		markAfter:  "526473", // the sync did not run
	}
	_, err := primer(p).Prime(context.Background(), "t/prime")
	if !errors.Is(err, ErrPrimeDidNotRun) {
		t.Fatalf("got %v, want ErrPrimeDidNotRun: the mark did not move, so nothing "+
			"ran, and reporting success would make this capability unfalsifiable", err)
	}
}

// TestARefreshThatRanWithNothingToDoIsSuccess is the other side, and both are
// needed: on a roster that is already current the numbers do not move, and that
// must remain a healthy outcome. The mark is what separates it from the case
// above.
func TestARefreshThatRanWithNothingToDoIsSuccess(t *testing.T) {
	compressPrimeClock(t)
	same := wireSnapshot{Total: 944, WithPushname: 456, LidWithPhone: 421, SyncPending: 31}
	got, err := primer(&primeDouble{
		before: same, after: same,
		markBefore: "526473", markAfter: "549772", // it ran
	}).Prime(context.Background(), "t/prime")
	if err != nil {
		t.Fatalf("a refresh that ran and found nothing produced an error: %v", err)
	}
	if !got.Ran() {
		t.Fatalf("Ran()=false while the mark moved: %s", got)
	}
	if got.Changed() {
		t.Fatalf("Changed()=true for identical snapshots: %s", got)
	}
	if !strings.Contains(got.String(), "ran=true") {
		t.Fatalf("the rendering hides the only proof there is: %s", got)
	}
}

// TestAnAbsentMarkIsNotProofOfARun: a page that cannot report the mark at all
// must not be read as a successful refresh. Empty is not "moved".
func TestAnAbsentMarkIsNotProofOfARun(t *testing.T) {
	compressPrimeClock(t)
	same := wireSnapshot{Total: 10}
	_, err := primer(&primeDouble{
		before: same, after: same,
		markBefore: "", markAfter: "549772",
	}).Prime(context.Background(), "t/prime")
	if !errors.Is(err, ErrPrimeDidNotRun) {
		t.Fatalf("got %v — a mark that was missing beforehand cannot show movement", err)
	}
}

// TestTheDidNotRunCheckComesBeforeTheShrinkCheck. A refresh that never executed
// cannot have damaged anything, and reporting ErrRosterShrank for it would send
// the reader after the wrong failure.
func TestTheDidNotRunCheckComesBeforeTheShrinkCheck(t *testing.T) {
	compressPrimeClock(t)
	_, err := primer(&primeDouble{
		before:     wireSnapshot{Total: 944},
		after:      wireSnapshot{Total: 900},
		markBefore: "526473", markAfter: "526473",
	}).Prime(context.Background(), "t/prime")
	if !errors.Is(err, ErrPrimeDidNotRun) {
		t.Fatalf("got %v, want ErrPrimeDidNotRun — the sync never ran, so the "+
			"smaller roster is not damage it caused", err)
	}
	if errors.Is(err, ErrRosterShrank) {
		t.Fatal("the shrink check ran first and named the wrong failure")
	}
}
