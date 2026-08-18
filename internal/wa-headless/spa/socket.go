package spa

// The socket-state signal, and the OPENING-duration threshold built on it.
//
// This is a THIRD liveness axis, next to the two this package already has:
//
//	page.go/probe.go answer "what page is this" from structure and markers.
//	liveness.go answers "did the round trip come back" from a probe deadline.
//	This file answers "how long has Meta's own socket enum sat in OPENING",
//	which is the question EVIDENCIA-SPA.md M6/M7/M8 exist to measure.
//
// It does not replace either of the other two, and it is not wired into
// spa.Monitor here: LOOP 04.4 introduces the signal and its threshold; wiring
// it into a consumer that can act on it is a later capability. Nothing in
// this repository imports this package yet (verified against the Chief's
// contract trace before writing a line of this file), so there is no
// existing caller whose behaviour this file could be changing.
//
// Study origin for the socket enum itself and its OPENING fallback:
// EVIDENCIA-SPA.md M4/M5 (F-21: a mounted UI over a dead socket answers
// Alive=true), M6 (OPENING during a healthy boot), M7 (OPENING's tail under
// CPU/network adversity), M8 (OPENING's permanence under a real cut).

import "time"

// SocketState is Meta's own connection-state enum for the WhatsApp Web
// socket, read from WAWebSocketModel.Socket.__x_state. It is Meta's
// vocabulary, not ours: the only values this package has ever observed
// against the real SPA are CONNECTED and OPENING (EVIDENCIA-SPA.md M4-M8).
// Anything else is preserved as read, not rejected — a third value showing
// up is exactly the kind of thing ClassifyOpeningDuration must not have an
// opinion about, because nothing here has measured it.
type SocketState string

const (
	// SocketStateConnected is a live, working socket.
	SocketStateConnected SocketState = "CONNECTED"
	// SocketStateOpening is BOTH a healthy boot passing through on its way
	// to CONNECTED (M6, M7) and a socket retrying against a server that is
	// gone (M5, M8) — the same string, and duration is the only thing that
	// tells them apart. That ambiguity is the entire reason C exists.
	SocketStateOpening SocketState = "OPENING"
	// SocketStateUnread is what socketStateReadExpr answers when the module
	// is not reachable — window.require threw, or the socket object had no
	// __x_state. It is the expression's own "I don't know", not a state
	// Meta ever reports, and it is empty on purpose: it is what an
	// interrupted or too-early read already produces without special-casing.
	SocketStateUnread SocketState = ""
)

// socketStateReadExpr is the ONE place this package reads Meta's socket
// state enum. It is a bare expression, not a statement wrapped in
// JSON.stringify, so a caller can embed it inside a larger script the way
// realspa_test.go's readinessScript and the long-cut trail recorder already
// do — those two call sites read the exact string this constant holds
// (verified against internal/wa-headless/realspa_test.go, which is out of
// this package's write set and therefore not edited to reference this
// constant; that consolidation is for whoever next touches that file).
//
// Two spellings of the same read diverging invisibly is a named hazard in
// this repository (realspa_test.go's own comment above socketStateReadJS)
// and this constant is what makes ONE spelling possible for production code:
// any future production reader of the socket enum embeds THIS, not a copy.
//
// It never sends anything, and it can never carry anything about the
// account: sk.__x_state is Meta's own connection-state word, not a value a
// page author or a contact ever wrote.
const socketStateReadExpr = `((() => {
	try {
		const m = window.require('` + string(ModuleSocketModel) + `');
		const sk = m && (m.Socket || m.default || m);
		return (sk && typeof sk.__x_state === 'string') ? sk.__x_state : '';
	} catch (e) { return ''; }
})())`

// SocketStateReadExpr exposes socketStateReadExpr to a future caller outside
// this file. It stays a raw expression (not JSON.stringify-wrapped) so it
// can be embedded inside a larger script the same way the two existing
// call sites in realspa_test.go do.
const SocketStateReadExpr = socketStateReadExpr

// SocketLiveness is what a duration spent in SocketStateOpening supports
// concluding, and NOTHING wider: "is this OPENING still inside the envelope
// M7 measured healthy boots to sit in, or has it crossed the derived
// threshold". It is NOT "is the session healthy" — CONNECTED is not a value
// this type has an opinion about, and a session can be perfectly fine while
// this type reads SocketOpeningDegraded (M8.4: three recoveries in
// 2.01-5.02s after sitting DEGRADED for a 12-minute cut).
//
// A WIDER taxonomy — BOOTSTRAPPING, RECOVERING, UNRESPONSIVE, SESSION_LOST —
// was named here in LOOP 04.4 as a taxonomy this package could grow into.
// The orchestration review of 276c131 rejected that: none of those four had
// a producer, and an executable type with unreachable placeholder values is
// a fictional contract — "preparing the future" is not something a pure
// duration->verdict function can do, because those four states all need
// either session HISTORY (has this socket ever reached CONNECTED? was the
// previous classification DEGRADED?) or INDEPENDENT evidence this file does
// not read (a page-level signal, a probe timeout). LOOP 04.5 removed them
// from the executable type; the taxonomy itself is not lost, it lives in
// EVIDENCIA-SPA.md and in CAP-06's still-unopened scope (the caller that
// tracks history and combines axes). Until that caller exists:
//
//	SESSION_LOST DETECTOR: NOT IMPLEMENTED. Explicit absence beats a false
//	detector — DEC-04.4-02 forbids duration in OPENING alone from ever
//	producing a session-lost verdict, and removing the placeholder value
//	makes that impossible to violate by construction, not just by policy.
type SocketLiveness string

const (
	// SocketOpeningWithinEnvelope means a duration spent in
	// SocketStateOpening is still below C, the envelope M7 measured healthy
	// boots to sit inside. It says nothing about CONNECTED, nothing about
	// session history, and nothing about whether the session is "healthy"
	// in any wider sense than this one axis.
	SocketOpeningWithinEnvelope SocketLiveness = "OPENING_WITHIN_ENVELOPE"

	// SocketOpeningDegraded means a duration spent in SocketStateOpening is
	// at or above C. This is the ONLY other state ClassifyOpeningDuration
	// can produce — see its doc comment and DEC-04.4-02 below. It does NOT
	// mean the session is lost: M8.4 measured this exact axis sitting
	// DEGRADED for at least 677.69s and then recovering in 2.01-5.02s with
	// no independent evidence the session was ever gone.
	SocketOpeningDegraded SocketLiveness = "OPENING_DEGRADED"
)

// The three terms below are DERIVED, term by term, from EVIDENCIA-SPA.md —
// not chosen — and are named data here (not just prose) so a change to one
// without a matching change to the others, or to their composition, fails a
// test instead of drifting silently (H14; see socket_test.go).
//
//	term 1 — healthyUpperBound = 1.36s
//	  The worst OPENING window measured across 21 boots over 7 conditions
//	  (unstressed, 3 CPU-contention levels, 3 network-degradation levels),
//	  all against the real, paired profile (M7.3). It is the `net-heavy`
//	  leg, round 1: 900ms of added latency — and in this dataset that leg
//	  is ALSO the worst boot overall (M7.3's table: the next highest is
//	  net-moderate at 0.76s, then the CPU legs, all below net-heavy). The
//	  two readings agree here, so this term is not a choice between two
//	  competing candidates. What the anchoring to `net-heavy` specifically
//	  (rather than "whichever leg is worst this time") buys is a claim
//	  about which AXIS to trust on a future re-measurement: M7.4
//	  established — with a named-before-the-run falsification test that
//	  the CPU axis FAILED — that this window tracks added LATENCY, not
//	  boot slowness or CPU contention. cpu-2x hit 22.86x dilation and
//	  still produced a SMALLER window than baseline. So if a re-measurement
//	  ever produces a worse "worst boot overall" than `net-heavy`'s, the
//	  axis that moved it is what should decide whether term 1 changes, not
//	  the raw maximum by itself.
//
//	term 2 — measurement_uncertainty = 0.31s
//	  M7.5's declared instrument resolution for the net-* legs specifically
//	  — the legs that produced term 1. Requested tick was 50ms; OBSERVED
//	  worst spacing on the net-* legs was 0.30-0.31s (M7.5's table), taken
//	  as 0.31s here (the worse of the two). M7.5 also shows this gap can
//	  shrink the recorded window as easily as inflate it (a gap covering
//	  the START shrinks; a gap covering the END inflates), and rules out
//	  the shrink case for these specific legs by the two-anchor delta
//	  argument — but "measured with this instrument's worst spacing" still
//	  means the recorded 1.36s carries up to this much slop, so it is
//	  carried forward as a term rather than assumed away.
//
//	term 3 — explicit_guard_band = 1.36s (== term 1)
//	  An explicit engineering choice, not a further measurement, and its
//	  rationale is stated because M7.8 item 1 requires one: every leg in
//	  M7, including net-heavy, is n=3 — "one condition sampled three
//	  times" (M7.8's own words), not a distribution with a tail this
//	  document can quote a percentile from. The chosen band is ONE MORE
//	  full width of the worst measured window: it doubles the signal
//	  instead of adding a number with no measurement behind it at all
//	  ("5s because it seems safe" is the explicitly rejected alternative).
//	  It is deliberately anchored to the SAME measured number as term 1
//	  rather than to an independent guess, and it is cheap to afford: M8.4
//	  found no ceiling below 677.69s, so an extra 1.36s spends a
//	  negligible fraction of that headroom, and M8.7 shows the dominant
//	  cost this threshold sits inside of is the 31.1-42.3s of DETECTION
//	  latency it gets ADDED to, not multiplied by.
//
//	C = healthyUpperBound + measurementUncertainty + explicitGuardBand
//	  = 1.36s + 0.31s + 1.36s = 3.03s
//
// INSTRUMENT RESOLUTION, declared next to C as the acceptance criteria
// require: the derivation above is built entirely from a 50ms-tick
// instrument whose OBSERVED worst spacing, on the legs that decided C, was
// 0.30-0.31s (M7.5) — four times finer than the M6 instrument's 250ms tick
// that this package's OWN OpeningWindowThreshold does NOT use as a term,
// because M7 superseded it without rebaselining the M6 numbers (M7.2).
//
// VALIDITY ENVELOPE: this derivation is valid ONLY up to 900ms of ADDED
// LATENCY — the ceiling of M7's three-point network curve (150 / 400 / 900ms
// -> 0.70 / 0.76 / 1.36s, M7.6). The curve responds monotonically to added
// latency inside that range; M7 never measured a link past 900ms (satellite,
// congested cellular, a slow captive portal), and M6.4/M7.6 both name
// extrapolating a three-point curve past its measured range as the exact
// "choice of table dressed as data" this repository's rule forbids. Beyond
// 900ms of added latency, whether C still separates a healthy boot from a
// dead session is UNKNOWN, not "probably fine" — this file does not soften
// that into a guess.
//
// BOUNDARY, at exactly C: inclusive. duration >= C is SocketOpeningDegraded,
// duration < C is SocketOpeningWithinEnvelope. Declared, not accidental:
// OPENING_DEGRADED never tears anything down (DEC-04.4-02, enforced
// structurally below), so the cost of counting the boundary sample as
// DEGRADED rather than WITHIN_ENVELOPE is nothing more than flagging one
// sample early — the asymmetric cost that would justify the opposite choice
// (treating C as an exclusive floor, i.e. waiting for one sample past it)
// does not exist here.
const (
	// healthyUpperBound is term 1 of C, see the derivation comment above.
	healthyUpperBound = 1360 * time.Millisecond
	// measurementUncertainty is term 2 of C, see the derivation comment above.
	measurementUncertainty = 310 * time.Millisecond
	// explicitGuardBand is term 3 of C, see the derivation comment above.
	explicitGuardBand = 1360 * time.Millisecond

	// OpeningWindowThreshold ("C" in EVIDENCIA-SPA.md M6/M7/M8 and the task
	// packet) is how long a socket may sit in OPENING before that duration
	// alone is worth flagging as SocketOpeningDegraded. Composed from the
	// three named terms above, not asserted equal to them — see
	// socket_test.go for the test that locks the composition.
	OpeningWindowThreshold = healthyUpperBound + measurementUncertainty + explicitGuardBand
)

// ClassifyOpeningDuration decides what a duration spent in SocketStateOpening
// means relative to C, and ONLY that. It answers exactly one question — "has
// this duration crossed the derived threshold" — and structurally cannot
// answer any other: its return type has exactly two values, and neither this
// function nor anything it calls holds a reference to a session-lost
// concept. DEC-04.4-02 is frozen precisely because M8 measured a socket
// sitting in OPENING for at least 677.69s (M8.4) and then recovering in
// 2.01-5.02s (M8.4) with no independent evidence the session was ever lost —
// duration in OPENING alone cannot tell "still retrying" from "gone", so
// this function does not pretend it can, and it does not claim anything
// about whether the SESSION is healthy — only about this one duration
// relative to the envelope M7 measured.
//
// The caller is responsible for calling this ONLY when the socket is
// currently read as SocketStateOpening; a duration spent CONNECTED is not
// this function's concern, and asking it about one is a caller error this
// package does not try to detect from inside a pure duration->verdict
// mapping.
func ClassifyOpeningDuration(d time.Duration) SocketLiveness {
	if d >= OpeningWindowThreshold {
		return SocketOpeningDegraded
	}
	return SocketOpeningWithinEnvelope
}
