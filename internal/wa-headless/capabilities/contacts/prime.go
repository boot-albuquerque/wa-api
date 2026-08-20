package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"wa-api/internal/wa-headless/engine"
	"wa-api/internal/wa-headless/spa"
)

// This file answers the product's primeContactRoster.
//
// IT IS THE ONE CAPABILITY WITH NO REFERENCE TO COPY. whatsapp-web.js has no
// equivalent, so nothing here came from reading someone else's solution — it
// came from measuring this build, and the measurement is what decided the
// shape.
//
// WHAT THE MEASUREMENT SAID, and it is not what the name promises:
//
//	before  944 contacts · getName 1 · pushname 456 · verifiedName 52
//	after   944 contacts · getName 1 · pushname 456 · verifiedName 56
//	cost    41.9 SECONDS, returning undefined
//
// And, measured first because it was the cheaper question: there is no
// membership gap to close. Of 391 contacts the chat collection references
// through the page's own getIsEligibleForContactSync rule, 391 were already in
// the roster. Nothing is missing.
//
// So doFullContactSync is a REFRESH, not a fetch. It does not add people, and
// it cannot invent address-book names the primary device does not have — which
// is why getName answers for 1 of 944 before and after. A capability that
// promised a primed roster would be promising what the mechanism does not
// deliver, so this one reports the before and after and lets the caller see for
// themselves.
//
// THE POSTCONDITION THAT MATTERS IS THE ONE AGAINST DAMAGE. This is a mutating
// call — the sync family writes through LidAwareContactsDB.bulkCreateOrMerge —
// and the failure worth catching is not "nothing improved" but "the roster came
// back SMALLER". A refresh that loses contacts is worse than no refresh, and it
// is the only outcome here that a caller could not detect on their own.

// Bounds for the refresh. Var, not const, so tests can compress the clock;
// production never assigns them.
//
// The budget is generous on purpose: the measured call took 41.9s, so anything
// near a minute would be a deadline set below the observed cost — the shape
// that makes a working operation look broken under load.
var (
	primeBudget = 5 * time.Minute
	primeTick   = 2 * time.Second
)

// syncMarkKey is the localStorage entry the refresh REGENERATES on every run,
// and it is what makes this capability's success claim falsifiable.
//
// It is not a configuration value, despite the name. Measured 2026-08-20 with
// the idle period as the control:
//
//	read                526473
//	after 45s IDLE      526473   <- does not drift on its own
//	after doFullContactSync   549772   <- moves
//
// That control is the reason this key can be trusted as a mark: "it changed
// after I called the sync" is not attribution, and every other candidate
// failed. isContactSyncCompleted holds 31 rows permanently pending, before and
// after; CONTACT_CHECKSUM is absent in both. Diffing the whole of localStorage
// across a run is what surfaced this one.
//
// Residual risk, stated rather than hidden: a background refresh landing inside
// our window would move it too. The idle control bounds that at 45 seconds of
// observed stillness, not at zero.
const syncMarkKey = "contact-sync-refresh-seconds"

var (
	// ErrPrimeDidNotRun is the postcondition that keeps this capability
	// honest: the page returned without error and the refresh mark did NOT
	// move, so the sync did not execute.
	//
	// Without it, "the roster was already current" and "the sync never ran"
	// are the same observation, and a doFullContactSync replaced by a no-op
	// would be reported as a healthy refresh. A detector that cannot fail is
	// not a detector.
	ErrPrimeDidNotRun = fmt.Errorf("contacts: the refresh returned but the page's sync mark did not move")
	// ErrPrime is the page refusing or throwing.
	ErrPrime = fmt.Errorf("contacts: the page refused the roster refresh")
	// ErrRosterShrank is the postcondition, and it is the reason this
	// capability verifies anything at all: a refresh that returns fewer
	// contacts than it started with has destroyed data, and the caller has no
	// other way to find out.
	ErrRosterShrank = fmt.Errorf("contacts: the roster SHRANK during the refresh")
)

// Snapshot is the roster measured at one instant. The fields are the ones whose
// emptiness the measurement found surprising, so a caller can see what a
// refresh actually moved.
type Snapshot struct {
	Total            int
	WithPushname     int
	WithName         int
	WithVerifiedName int
	// LidWithPhone is how many "@lid" rows carry a phoneNumber, and it is the
	// number that actually MOVES.
	//
	// The first version of this snapshot did not have it, and the live proof
	// exposed the omission: PrimeResult reported changed=false while the roster
	// read 521 people from 944 rows where it had read 544 from the same 944.
	// More lid rows had gained a phone, so more of them merged — the refresh
	// had done something material and this capability called it "nothing".
	//
	// It is the right number to watch because the lid -> phone edge is the ONLY
	// cross-identity link this build provides (0 of 454 phone rows carry a lid,
	// see H39), so it is what deduplication depends on.
	LidWithPhone int
	// SyncPending is how many rows the page has NOT marked
	// isContactSyncCompleted, and it is the observable proof that a refresh
	// actually ran.
	//
	// Without it this capability could not fail: "the roster was already
	// current" and "the sync never executed" both show up as nothing changing,
	// so a doFullContactSync replaced by a no-op would be reported as healthy.
	// A detector that cannot distinguish those is not a detector.
	//
	// Measured on the lab profile: 944 rows all carry the field, 913 marked
	// completed, 31 pending.
	SyncPending int
}

func (s Snapshot) String() string {
	return fmt.Sprintf("total=%d pushname=%d name=%d verified=%d lidWithPhone=%d pending=%d",
		s.Total, s.WithPushname, s.WithName, s.WithVerifiedName, s.LidWithPhone, s.SyncPending)
}

// PrimeResult is what a refresh did, stated as a difference.
type PrimeResult struct {
	Before Snapshot
	After  Snapshot
	// Waited is the measured cost. It is carried because it is large — 41.9s
	// when measured — and a caller budgeting around this needs the number
	// rather than an assumption.
	Waited time.Duration

	// The refresh mark, unexported because its VALUE means nothing to a caller
	// — only whether it moved, which Ran() answers.
	markBefore, markAfter string
}

// Added is how many contacts appeared. Measured at zero on a profile with no
// membership gap, which is the ordinary case rather than a failure.
func (r PrimeResult) Added() int { return r.After.Total - r.Before.Total }

// Ran reports whether the page's own refresh mark moved.
//
// It is the difference between "there was nothing to do" and "nothing was
// done", and it is the only evidence available for that distinction: the sync
// returns undefined, so its return value says nothing at all.
func (r PrimeResult) Ran() bool {
	return r.markBefore != "" && r.markAfter != "" && r.markBefore != r.markAfter
}

// Linked is how many more "@lid" rows learned their phone number.
//
// This is the useful number, and the one worth reporting to a caller who wants
// to know whether the refresh was worth its 42 seconds: every new link is one
// person who stops being counted twice by List.
func (r PrimeResult) Linked() int { return r.After.LidWithPhone - r.Before.LidWithPhone }

// Changed reports whether the refresh moved ANY of the measured numbers. False
// is a legitimate and common outcome: it means the roster was already current.
func (r PrimeResult) Changed() bool { return r.Before != r.After }

func (r PrimeResult) String() string {
	return fmt.Sprintf("contacts.PrimeResult(ran=%t added=%d linked=%d changed=%t waited=%s | before: %s | after: %s)",
		r.Ran(), r.Added(), r.Linked(), r.Changed(), r.Waited.Round(time.Millisecond), r.Before, r.After)
}

const primeStateKey = "__waHeadlessContactPrime"

type wireSnapshot struct {
	Total            int `json:"total"`
	WithPushname     int `json:"with_pushname"`
	WithName         int `json:"with_name"`
	WithVerifiedName int `json:"with_verified_name"`
	LidWithPhone     int `json:"lid_with_phone"`
	SyncPending      int `json:"sync_pending"`
}

func (w wireSnapshot) toSnapshot() Snapshot {
	return Snapshot{
		Total:            w.Total,
		WithPushname:     w.WithPushname,
		WithName:         w.WithName,
		WithVerifiedName: w.WithVerifiedName,
		LidWithPhone:     w.LidWithPhone,
		SyncPending:      w.SyncPending,
	}
}

type wirePrime struct {
	Stage      string       `json:"stage"`
	OK         bool         `json:"ok"`
	Why        string       `json:"why"`
	Before     wireSnapshot `json:"before"`
	After      wireSnapshot `json:"after"`
	MarkBefore string       `json:"mark_before"`
	MarkAfter  string       `json:"mark_after"`
}

func primeKickScript() string {
	return `JSON.stringify((() => {
		window[` + strconv.Quote(primeStateKey) + `] = { stage: 'pending', ok: false, why: '' };
		const park = (v) => { window[` + strconv.Quote(primeStateKey) + `] = v; };
		const snap = () => {
			const CC = window.require('` + string(spa.ModuleContactCollection) + `').ContactCollection;
			const G = window.require('` + string(spa.ModuleContactGetters) + `');
			const all = CC.getModelsArray();
			let name = 0, pushname = 0, verified = 0, lidWithPhone = 0, pending = 0;
			for (const c of all) {
				try {
					if (G.getName && G.getName(c)) { name++; }
					if (G.getPushname && G.getPushname(c)) { pushname++; }
					if (G.getVerifiedName && G.getVerifiedName(c)) { verified++; }
					// The lid -> phone edge, which is what the refresh moves.
					if (c.id && c.id.server === 'lid' && c.phoneNumber && c.phoneNumber.user) {
						lidWithPhone++;
					}
					// The page's own completion mark, which is what proves a
					// refresh executed rather than merely returned.
					if (!c.isContactSyncCompleted) { pending++; }
				} catch (e) {}
			}
			return { total: all.length, with_pushname: pushname,
				with_name: name, with_verified_name: verified,
				lid_with_phone: lidWithPhone, sync_pending: pending };
		};
		const mark = () => {
			try { return String(localStorage.getItem(` + strconv.Quote(syncMarkKey) + `)); }
			catch (e) { return ''; }
		};
		(async () => {
			let stage = 'snapshot';
			try {
				const before = snap();
				const markBefore = mark();
				stage = 'sync';
				const B = window.require('` + string(spa.ModuleContactSyncBridge) + `');
				// It returns undefined; there is nothing to inspect in the
				// answer, which is exactly why the postcondition is measured
				// from the collection rather than read from a return value.
				await B.doFullContactSync();
				stage = 'verify';
				park({ stage: 'done', ok: true, why: '', before: before, after: snap(),
					mark_before: markBefore, mark_after: mark() });
			} catch (e) {
				park({ stage, ok: false, why: String((e && e.message) || e).slice(0, 160) });
			}
		})();
		return { started: true };
	})())`
}

const primePollScript = `JSON.stringify((() => {
	const s = window[` + `"` + primeStateKey + `"` + `];
	if (!s) { return { stage: 'sync', ok: false, why: 'STATE_MISSING' }; }
	return s;
})())`

// Prime asks the page to refresh the roster and reports what moved.
//
// It returns an error only when the page refused or when the roster SHRANK.
// "Nothing changed" is a successful result with Changed() false, because on a
// roster that is already current that is the correct answer — and the measured
// ordinary case.
func (l *Lister) Prime(ctx context.Context, label string) (PrimeResult, error) {
	start := time.Now()

	var kicked string
	if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/kick", func(ctx context.Context) error {
		return l.eval(ctx, primeKickScript(), &kicked)
	}); err != nil {
		return PrimeResult{}, fmt.Errorf("%w: %v", ErrPrime, err)
	}

	deadline := time.Now().Add(primeBudget)
	var w wirePrime
	for {
		var raw string
		if err := l.runner.Do(ctx, engine.OpStateProbe, label+"/poll", func(ctx context.Context) error {
			return l.eval(ctx, primePollScript, &raw)
		}); err != nil {
			return PrimeResult{}, fmt.Errorf("%w: %v", ErrPrime, err)
		}
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			return PrimeResult{}, fmt.Errorf("contacts: unexpected refresh answer: %w", err)
		}
		if w.Stage != "pending" {
			break
		}
		if !time.Now().Before(deadline) {
			return PrimeResult{}, fmt.Errorf("%w: the page never settled within %s "+
				"(the measured cost was 41.9s, so this is a stall and not slowness)",
				ErrPrime, primeBudget)
		}
		time.Sleep(primeTick)
	}
	if !w.OK {
		return PrimeResult{}, fmt.Errorf("%w at %s (%s)", ErrPrime, w.Stage, w.Why)
	}

	out := PrimeResult{
		Before:     w.Before.toSnapshot(),
		After:      w.After.toSnapshot(),
		Waited:     time.Since(start),
		markBefore: w.MarkBefore,
		markAfter:  w.MarkAfter,
	}
	// THE POSTCONDITION THAT MAKES SUCCESS FALSIFIABLE. Checked BEFORE the
	// shrink check, because a refresh that never ran cannot have damaged
	// anything and reporting it as intact would be the wrong sentence.
	if !out.Ran() {
		return PrimeResult{}, fmt.Errorf("%w (the command returned after %s and the "+
			"roster was left as it was found)", ErrPrimeDidNotRun, out.Waited.Round(time.Millisecond))
	}
	// THE POSTCONDITION. A refresh that lost contacts is the one outcome the
	// caller cannot detect for themselves, and it is worse than not refreshing.
	if out.After.Total < out.Before.Total {
		return PrimeResult{}, fmt.Errorf("%w: %d before, %d after",
			ErrRosterShrank, out.Before.Total, out.After.Total)
	}
	return out, nil
}
