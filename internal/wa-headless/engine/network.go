package engine

// Severing the PAGE's network, without touching the machine's.
//
// It exists for one measurement: a session whose UI is mounted and whose
// renderer answers, but whose server is gone. That is the failure mode the
// liveness probe explicitly does NOT yet rule out, and the only honest way to
// look at it is to produce it — a drop emulated at the browser is what happens
// when a laptop lid closes.
//
// It is NOT a logout, a signal or a session revocation. Nothing here unlinks an
// account, and nothing here writes to the profile: the emulation lives in the
// browser's memory and is undone by restoring it or by closing the browser.
//
// Like everything else that talks to the driver, it is confined to this package
// (ADR-0006 D1, enforced by ../gate_test.go).

import (
	"context"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	// throughputUnthrottled is the protocol's sentinel for "leave bandwidth
	// alone". Zero would be a hard zero-byte ceiling, which is a different
	// thing: it would throttle the ONLINE leg too and make the control
	// meaningless.
	throughputUnthrottled = -1
	// latencyUnthrottled adds no artificial delay, in milliseconds.
	latencyUnthrottled = 0
	// allRequests is the empty URL pattern the protocol reads as "every
	// request", which is how emulateNetworkConditionsByRule expresses a GLOBAL
	// condition rather than a per-URL one.
	allRequests = ""
)

// SetNetworkOffline severs the page's connectivity, or restores it.
//
// Two commands, because one of them alone would emulate half a network drop and
// the difference is measurable from inside the page:
//
//   - emulateNetworkConditionsByRule fails the REQUESTS. Its own documentation
//     says it "does not affect navigator state", so on its own the application
//     still believes it is online and never fires its offline handler.
//   - overrideNetworkState flips navigator.onLine and fires that handler, and on
//     its own it would be a lie: requests would keep succeeding, so an
//     application that reacts by reconnecting would reconnect successfully.
//
// A real drop is both, and measuring against half of one would produce a
// finding about our emulation instead of about the session — with one
// exception, which is deliberate and lives in SetTransportOffline below: when
// the question is whether the application notices BY ITSELF, the announcement
// has to be withheld.
//
// Network.enable comes first because the emulation commands live in that domain
// and are rejected without it. It also makes Chrome emit network events, which
// carry URLs: this package registers no listener for them, so they are
// discarded by the driver and never read, logged or persisted.
func (t *Tab) SetNetworkOffline(r *Runner, offline bool, label string) error {
	return t.emulateOffline(r, offline, true, label)
}

// SetTransportOffline kills the page's REQUESTS and leaves navigator.onLine
// alone — the half of SetNetworkOffline that emulateNetworkConditionsByRule
// provides, on its own and on purpose.
//
// It exists because the two halves answer different questions, and only this one
// answers the question that decides whether a socket state is usable in
// production.
//
// With both commands, an application that leaves CONNECTED could be reacting to
// the `offline` DOM event that overrideNetworkState fires — a reaction that
// exists only because the emulation announced itself. The failures a fleet
// actually meets announce nothing: a black-holed route, a dead upstream, a
// captive portal and a hung server all leave navigator.onLine TRUE and fire no
// event. This function reproduces THAT: the bytes stop, and nothing tells the
// page.
//
// So it is not a weaker sever. It is the stricter one — the only form in which
// "the application noticed" means the application noticed by itself.
//
// Restoring goes through the same path with offline=false, which clears the rule
// list; the navigator was never touched, so there is nothing there to restore.
func (t *Tab) SetTransportOffline(r *Runner, offline bool, label string) error {
	return t.emulateOffline(r, offline, false, label)
}

// emulateOffline is the one place the emulation commands are issued.
//
// withNavigatorState selects whether the page's own view of the network is
// flipped along with the traffic. Both callers are above; the split is a
// parameter rather than two bodies because the ordering, the Network.enable and
// the empty-list restore are the same in both, and a second copy is the same bug
// waiting to diverge.
func (t *Tab) emulateOffline(r *Runner, offline, withNavigatorState bool, label string) error {
	// Restoring is expressed as NO conditions rather than as a condition with
	// offline=false: the rule list is replaced wholesale, so an empty list is
	// the only form that leaves nothing behind.
	var conditions []*network.Conditions
	if offline {
		conditions = []*network.Conditions{{
			URLPattern:         allRequests,
			Offline:            true,
			Latency:            latencyUnthrottled,
			DownloadThroughput: throughputUnthrottled,
			UploadThroughput:   throughputUnthrottled,
		}}
	}

	// OpAction, because this is a command that changes the target's state and
	// the Action budget is what that class is budgeted at. It is not a probe:
	// its answer is not evidence about the page.
	return r.Do(t.ctx, OpAction, label, func(ctx context.Context) error {
		runCtx, cancel := t.derive(ctx)
		defer cancel()
		actions := []chromedp.Action{
			network.Enable(),
			chromedp.ActionFunc(func(ctx context.Context) error {
				// Wrapped because this command answers with rule IDs, so its Do
				// does not match the driver's Action signature. The IDs identify
				// the emulation rules; they are discarded because restoring
				// replaces the whole list.
				_, err := network.EmulateNetworkConditionsByRule(conditions).
					WithEmulateOfflineServiceWorker(offline).Do(ctx)
				return err
			}),
		}
		if withNavigatorState {
			actions = append(actions, network.OverrideNetworkState(offline,
				latencyUnthrottled, throughputUnthrottled, throughputUnthrottled))
		}
		return chromedp.Run(runCtx, actions...)
	})
}

// --- DEGRADING the page's network, which is NOT severing it ----------------
//
// LOOP 04.3E needs the leg where the mechanism WORSENS: how long a HEALTHY boot
// stays negotiating when the network is slow rather than gone. That is a
// different phenomenon from everything above, and the difference is not a
// nuance — it is the confound that LOOP 04.3A already paid for once.
//
// overrideNetworkState makes the browser fire an `offline` EVENT on the page,
// and EVIDENCIA-SPA.md M5.4 measured what that costs: the SPA reacts to the
// ANNOUNCEMENT in ~1.4-3.1s and to the actual stall in ~33-34s. A measurement
// of a slow network that accidentally announced an outage would be measuring
// the announcement.
//
// So the type below cannot express an outage. Offline is not a parameter of
// NetworkDegradation and is written false at the single point of construction:
// the two mechanisms stay distinct at the TYPE level, and a later edit cannot
// quietly turn a degradation call into an outage call. Making it impossible is
// worth more than documenting it, because this repository has already measured
// the difference between a comment and a mechanism.
//
// The duplication with emulateOffline above is DELIBERATE for the same reason.
// A shared body would be exactly the mutable path through which one call could
// become the other, and eight lines are a cheap price for the two never being
// able to meet.

// NetworkDegradation is a slow network, never an absent one.
//
// Zero values mean UNTHROTTLED on each axis independently, so a partially
// filled struct degrades only what it names rather than silently clamping the
// rest to zero bytes — which is an outage wearing a throughput's costume.
type NetworkDegradation struct {
	// Latency is the delay added to every request, rounded to milliseconds,
	// which is the protocol's own unit.
	Latency time.Duration
	// DownloadBytesPerSecond and UploadBytesPerSecond cap throughput.
	DownloadBytesPerSecond float64
	UploadBytesPerSecond   float64
}

// degradedConditions builds the rule list, and is separate from the method so
// that the one property that matters about it can be asserted without a
// browser. Its guard is TestDegradedConditionsCannotExpressAnOutage.
func degradedConditions(d NetworkDegradation) []*network.Conditions {
	throughput := func(bps float64) float64 {
		if bps <= 0 {
			return throughputUnthrottled
		}
		return bps
	}
	return []*network.Conditions{{
		URLPattern: allRequests,
		// The invariant of this whole file half. Not a parameter, not derived
		// from the argument, not reachable from outside: a degraded network is
		// a network that is THERE.
		Offline:            false,
		Latency:            float64(d.Latency.Milliseconds()),
		DownloadThroughput: throughput(d.DownloadBytesPerSecond),
		UploadThroughput:   throughput(d.UploadBytesPerSecond),
	}}
}

// SetNetworkDegraded makes the page's network SLOW. It never makes it absent:
// navigator.onLine stays true, no `offline` event is fired, and every request
// still completes — late, and at a lower rate.
//
// WithEmulateOfflineServiceWorker is deliberately NOT sent: it is the outage
// half of the protocol, and this call has no outage to express.
//
// Clearing goes through ClearNetworkConditions.
func (t *Tab) SetNetworkDegraded(r *Runner, d NetworkDegradation, label string) error {
	return t.applyConditions(r, degradedConditions(d), label)
}

// clearedConditions builds the rule list of the RESTORE path, and is separate
// from the method for the same reason degradedConditions is: the one property
// that matters about it — that it is EMPTY — can then be asserted without a
// browser. Its guard is TestClearNetworkConditionsClearsWithAnEmptyRuleList.
//
// An EMPTY list rather than a rule saying "unthrottled": the rule list is
// replaced wholesale, so an empty one is the only form that leaves nothing
// behind. A rule that named every axis as unthrottled would still be a rule,
// and the next edit to it would be an emulation nobody asked for.
func clearedConditions() []*network.Conditions { return nil }

// ClearNetworkConditions removes every emulation rule.
//
// The list it sends is clearedConditions() — empty, for the reason stated
// there. The navigator was never touched by this half of the file, so there is
// nothing there to restore either.
func (t *Tab) ClearNetworkConditions(r *Runner, label string) error {
	return t.applyConditions(r, clearedConditions(), label)
}

// applyConditions issues the rule list under the Action budget.
//
// OpAction for the same reason emulateOffline uses it: this is a command that
// changes the target's state, not a probe whose answer is evidence. Going
// through the Runner is what keeps invariant 5 — no CDP path blocks
// indefinitely, deadline per operation class, on the Go side — true of the new
// path as well as the old one.
func (t *Tab) applyConditions(r *Runner, conditions []*network.Conditions, label string) error {
	return r.Do(t.ctx, OpAction, label, func(ctx context.Context) error {
		runCtx, cancel := t.derive(ctx)
		defer cancel()
		return chromedp.Run(runCtx,
			network.Enable(),
			chromedp.ActionFunc(func(ctx context.Context) error {
				_, err := network.EmulateNetworkConditionsByRule(conditions).Do(ctx)
				return err
			}),
		)
	})
}
