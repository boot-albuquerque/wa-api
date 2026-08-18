package spa

// Waiting for a page to finish becoming what it is going to be.
//
// core.StartSession used to call Probe/Classify exactly once, immediately
// after Navigate returned, and fail the boot if the page was not already
// APP_READY. Every fixture in this module's own test suite mounts
// #pane-side in its initial HTML, so that single probe always won against a
// double — and never against the real target, which the same repository had
// already measured taking seconds longer than navigation to finish mounting
// (HANDOFF-INICIATIVA.md, "o caminho de produção classifica uma vez"). The
// fixture was not more permissive than production; it was FASTER, and speed
// was the dimension that hid the defect.
//
// This file is the fix: a Go-side polling loop, under the caller's Runner and
// a bounded context, that keeps calling Probe/Classify until the page reaches
// a verdict this package considers final — success, or a class no amount of
// waiting will change.
//
// The boundary this file exists to hold: core/session.go should only have to
// know "wait for the readiness condition, with a budget". It should not know
// how many polls that costs, how long to sleep between them, or which classes
// are worth re-checking — that is exactly the kind of temporal knowledge
// spa/ already owns (Probe's own two-step evaluation is exactly this kind of
// decision), and duplicating it in core/ would be the boundary violation
// ADR-0006 D1 was written to prevent for the driver itself.

import (
	"context"
	"time"

	"wa-api/internal/wa-headless/engine"
)

// DefaultSettleBudget bounds how long WaitForReady will keep polling a page
// that has not yet reached a final verdict.
//
// This is an OPERATIONAL WAITING BUDGET, not a service-level agreement: it
// says how long this module is willing to keep a boot attempt open, not how
// long a real SPA is promised to take. HANDOFF §F2's own measured numbers
// (recovery p50 10.4s, app-ready observed out to 15.8s, three samples) were
// explicitly NOT treated as an SLA to derive a new budget from — the packet
// that requested this fix says so explicitly, because three samples is not
// evidence of a ceiling. The value below is instead the outer budget that
// was ALREADY running in the field the day this defect was measured: the
// boot that failed at T+4.7s had a 60s external context around it and still
// failed, because nothing was wrong with the budget — nothing was polling.
// 60s is comfortably above every F2 sample without pretending to be a
// derived percentile, in the same spirit as OpBoot's 30s over an 8.5s
// measured ceiling (engine/deadline.go): headroom, not an expectation.
//
// The caller's own context stays sovereign regardless: WaitForReady derives
// its working deadline from context.WithTimeout(ctx, budget), which yields
// whichever of the two expires first.
const DefaultSettleBudget = 60 * time.Second

// settlePollInterval spaces consecutive probes while a page has answered but
// not yet reached a final verdict.
//
// It has nothing to do with StateProbe's 5s per-call deadline (engine/
// deadline.go): that budget bounds ONE evaluation round trip, and would make
// this loop poll far too slowly if reused here. This interval is local to
// the settle loop, keeps it off the page's own clock (ARMADILHAS 19 /
// gate_test.go's TestNoWaitKeepsItsClockInThePage — this is a plain Go-side
// time.After, not chromedp.Poll or anything backed by a page timer), and was
// picked to be short next to lateMountDelay (6s) and DefaultSettleBudget
// (60s) without flooding the page with evaluations.
const settlePollInterval = 500 * time.Millisecond

// settleTerminal reports whether class is a verdict WaitForReady should stop
// on WITHOUT reaching APP_READY — a class no further waiting is expected to
// change, or one this restoration-only boot path is not allowed to wait out
// even if it might eventually change.
//
// Per-class reasoning, the decision table this fix is required to produce:
//
//   - ClassAppReady: not handled here — the caller returns as soon as it sees
//     this class, before ever asking settleTerminal.
//   - ClassOther: NOT terminal. This is the class a still-mounting page
//     reports (see page.go's Classify: no marker matched, page answered,
//     nothing recognised yet), and it is the entire reason this file exists.
//   - ClassPairingLoading: terminal. It means the pairing screen is already
//     mounted and the QR is only moments from rendering — the SESSION IS
//     GONE. session.go's own doc says this boot path is restoration-only and
//     never waits for a human; waiting the extra few seconds for the QR
//     canvas to appear would only convert this into ClassLoginRequired, which
//     is ALSO terminal here. Stopping at the earlier signal fails faster
//     without changing the outcome.
//   - ClassLoginRequired: terminal. Same restoration-only rule, explicit in
//     the class itself: "only a human with a phone can restore it"
//     (page.go). No pairing, no waiting for a person, from this path.
//   - ClassSessionConflict: terminal. PageClass.Terminal() already says so
//     (page.go: "WhatsApp took the session elsewhere... one active session
//     per profile, and the newest tab wins"). Another tab owns this profile
//     right now; waiting does not give it back, and continuing to poll would
//     just keep re-reading a page whose owner is elsewhere.
//   - ClassErrorPage: terminal. This is WhatsApp refusing the browser itself
//     (page.go: the "update Chrome" screen). Nothing about that state is a
//     function of elapsed time; a browser this page has already rejected
//     does not become acceptable to it by waiting.
//   - ClassRedirect: NOT terminal, and this was found empirically while
//     building this fix, not decided up front. Classify (page.go) reports
//     REDIRECT for ANY page that answers with a URL not containing
//     "web.whatsapp.com" and matched no structural marker — which is true of
//     every local httptest fixture in this test suite (including
//     lateMountingPage below) until the moment its own script mounts
//     #pane-side, because a local fixture server is never on that host in
//     the first place. Marking REDIRECT terminal made
//     TestStartSession_LateMountingSPA_SingleProbeFailsBeforePageFinishesMounting
//     fail on the FIRST probe with a REDIRECT verdict, before
//     lateMountDelay ever elapsed — exactly the single-shot behaviour this
//     fix exists to remove, just relabelled. Against the real target,
//     NavigateURL already IS web.whatsapp.com, so a genuine mid-boot
//     REDIRECT there means the SPA itself sent the page elsewhere (an actual
//     off-host bounce), which is rare enough that this repository has no
//     measurement of it settling back. Keeping it in the wait set costs
//     nothing new: it is still bounded by the very same overall budget as
//     OTHER, so a page that is truly redirected away and stays there still
//     fails cleanly once that budget runs out, it just does not fail on the
//     first look.
//   - ClassUnresponsive: NOT terminal. Probe/ClassifyProbe folds every probe
//     failure into this one class on purpose (probe.go: "a blown deadline, a
//     dropped CDP connection and a script that threw are different causes
//     with the same consequence"), and page.go's own doc is explicit that it
//     is "NEVER inferred from missing elements" — it is a statement about one
//     evaluation, not about the session. Phase 6 measured a page recovering
//     JavaScript execution after being unresponsive; treating one bad probe
//     as final would convert a transient stall into a boot failure. It keeps
//     being retried like OTHER, bounded by the same overall budget, so a
//     renderer that never recovers still fails cleanly once the budget runs
//     out rather than hanging.
func settleTerminal(c PageClass) bool {
	switch c {
	case ClassPairingLoading, ClassLoginRequired, ClassSessionConflict, ClassErrorPage:
		return true
	default:
		// ClassOther, ClassRedirect and ClassUnresponsive: keep waiting.
		return false
	}
}

// WaitForReady polls Probe/Classify until the page reaches APP_READY, a
// class settleTerminal considers final, or budget runs out — whichever comes
// first, with the caller's own ctx staying sovereign throughout.
//
// It always returns the LAST STRUCTURAL SNAPSHOT actually observed, even on
// exhaustion, so a caller building a failure report (core.BootFailure's
// StageNotReady branch names url and dom_nodes from exactly this value)
// still has real evidence rather than a zero value. This matters concretely
// on the exhaustion path: the probe that finally sees settleCtx expire
// always reports ClassUnresponsive with a ZERO snapshot (ClassifyProbe folds
// every probe error into that class before any JSON ever gets decoded —
// probe.go), so naively returning that probe's own snapshot would silently
// discard the last GOOD structural read in favour of an empty one. The class
// returned is still whatever that last probe actually was — Unresponsive is
// itself useful information — only the snapshot is carried forward from the
// last probe that actually decoded one.
func WaitForReady(ctx context.Context, r *engine.Runner, eval Evaluator, budget time.Duration, label string) (PageSnapshot, PageClass) {
	settleCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	var lastSnap PageSnapshot
	lastClass := ClassOther
	for {
		snap, class := Probe(settleCtx, r, eval, label)
		lastClass = class
		if class != ClassUnresponsive {
			// Classify (unlike ClassifyProbe) never returns Unresponsive, so
			// any other class here came from a snapshot that actually
			// decoded — safe to keep as the best evidence seen so far.
			lastSnap = snap
		}
		if class == ClassAppReady || settleTerminal(class) {
			return lastSnap, class
		}
		select {
		case <-settleCtx.Done():
			// Budget exhausted (or the caller's own ctx expired first, which
			// context.WithTimeout already folded into settleCtx). Return the
			// last thing actually observed rather than looping once more —
			// looping here would just spend one more probe past a deadline
			// that already expired.
			return lastSnap, lastClass
		case <-time.After(settlePollInterval):
		}
	}
}
