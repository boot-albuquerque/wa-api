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

// SocketLiveness is the taxonomy phase 6's rule widens into once duration is
// a signal: not just "did the probe answer" (spa.PageClass /
// spa.ClassifyProbe already own that), but "is this session's socket axis
// worth watching, worth escalating on, or neither".
//
// It is a WIDER lens than PageClass on purpose, and the two are not meant to
// collapse into each other: PageClass answers "what is this page right now"
// from one snapshot; SocketLiveness answers "what does this axis mean over
// time". CAP-06 is where a caller combines both with probe-failure evidence
// (spa.Monitor's ConsecutiveFailures) into one verdict a recycler can act on
// — that wiring is out of scope here (DO_NOT list, task packet LOOP-04.4-T1).
//
// Not every member is reachable from the code this file ships. That is
// documented per-constant below, and it is deliberate: the packet asks this
// package to be ABLE TO REASON about the full taxonomy, not to have a
// producer for every branch of it before the consumer exists.
type SocketLiveness string

const (
	// SocketHealthy is CONNECTED, or OPENING for a duration below C. Session
	// signal-wise, "nothing to watch here yet".
	SocketHealthy SocketLiveness = "HEALTHY"

	// SocketBootstrapping is OPENING on a session that has never reached
	// CONNECTED once. M6/M7 measured exactly this window — a healthy boot's
	// OPENING — and its distribution IS the healthy_upper_bound term C is
	// built from. This package does not yet track "has this session ever
	// seen CONNECTED", so nothing here produces SocketBootstrapping today;
	// it is named because a caller that DOES track that history (a future
	// Monitor field) needs a state to report instead of overloading
	// SocketHealthy for a session that is not yet proven healthy.
	SocketBootstrapping SocketLiveness = "BOOTSTRAPPING"

	// SocketDegraded is OPENING for a duration at or above C, on a socket
	// that HAD reached CONNECTED before. This is the ONLY state
	// ClassifyOpeningDuration can produce besides SocketHealthy — see its
	// doc comment and DEC-04.4-02 below.
	SocketDegraded SocketLiveness = "DEGRADED"

	// SocketRecovering is a socket that just left OPENING back to
	// CONNECTED after having been DEGRADED. M8.4 measured this transition
	// taking 2.01-5.02s after a 12-minute cut (three recoveries, all valid
	// runs) — fast, and the reason DEC-04.4-02 forbids any duration-only
	// path to SESSION_LOST: a session a C-based watcher would have killed
	// was, in all three measured cases, seconds from coming back on its
	// own. Nothing in this file produces this state either: it needs the
	// PREVIOUS classification, which is a caller's job (a stateful
	// Monitor), not this stateless function's.
	SocketRecovering SocketLiveness = "RECOVERING"

	// SocketUnresponsive is spa.ClassUnresponsive's axis, restated in this
	// vocabulary for a caller that reasons about both at once: the PROBE
	// round trip did not come back within its budget (liveness.go,
	// DefaultUnresponsiveAfter). It is a different measurement than
	// anything in this file — it does not read the socket enum at all —
	// and ClassifyOpeningDuration never produces it. Listed here so a
	// consumer combining both axes has one vocabulary instead of two.
	SocketUnresponsive SocketLiveness = "UNRESPONSIVE"

	// SocketSessionLost means the session is gone and only re-pairing
	// restores it. DEC-04.4-02, frozen by the initiative's technical
	// direction: duration in OPENING can NEVER, by itself, produce this
	// value. M8 measured the socket sitting in OPENING for at least
	// 677.69s under a real cut with no ceiling inside the 12-minute
	// measurement window (EVIDENCIA-SPA.md M8.4, M8.7) — there is no
	// duration past which OPENING alone distinguishes "the session is
	// gone" from "the network is still gone but the session would come
	// right back", because the M8 control never saw the socket LEAVE
	// OPENING for anything but CONNECTED. Reaching this value requires
	// evidence this file does not have: a page-level signal (QR shown,
	// "used in another window" — spa.ClassLoginRequired,
	// spa.ClassSessionConflict) or an explicit revocation this package has
	// never measured (EVIDENCIA-SPA.md M8.8 item 5). No function in this
	// file has a branch that returns SocketSessionLost — see
	// TestClassifyOpeningDurationNeverReachesSessionLost's negative
	// control for the executed proof.
	SocketSessionLost SocketLiveness = "SESSION_LOST"
)

// OpeningWindowThreshold ("C" in EVIDENCIA-SPA.md M6/M7/M8 and the task
// packet) is how long a socket may sit in OPENING before that duration alone
// is worth flagging as SocketDegraded.
//
// It is DERIVED, term by term, from EVIDENCIA-SPA.md — not chosen:
//
//	term 1 — healthy_upper_bound = 1.36s
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
//	C = term1 + term2 + term3 = 1.36s + 0.31s + 1.36s = 3.03s
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
// BOUNDARY, at exactly C: inclusive. duration >= C is SocketDegraded,
// duration < C is SocketHealthy. Declared, not accidental: DEGRADED never
// tears anything down (DEC-04.4-02, enforced structurally below), so the
// cost of counting the boundary sample as DEGRADED rather than HEALTHY is
// nothing more than flagging one sample early — the asymmetric cost that
// would justify the opposite choice (treating C as an exclusive floor, i.e.
// waiting for one sample past it) does not exist here.
const OpeningWindowThreshold = 3030 * time.Millisecond

// ClassifyOpeningDuration decides what a duration spent in SocketStateOpening
// means, and ONLY that. It answers exactly one question — "has this
// duration crossed the derived threshold" — and structurally cannot answer
// any other: its return type has two reachable values, SocketHealthy and
// SocketDegraded, and neither this function nor anything it calls holds a
// reference to SocketSessionLost. DEC-04.4-02 is frozen precisely because
// M8 measured a socket sitting in OPENING for at least 677.69s (M8.4) and
// then recovering in 2.01-5.02s (M8.4) with no independent evidence the
// session was ever lost — duration in OPENING alone cannot tell "still
// retrying" from "gone", so this function does not pretend it can.
//
// The caller is responsible for calling this ONLY when the socket is
// currently read as SocketStateOpening; a duration spent CONNECTED is not
// this function's concern, and asking it about one is a caller error this
// package does not try to detect from inside a pure duration->verdict
// mapping.
func ClassifyOpeningDuration(d time.Duration) SocketLiveness {
	if d >= OpeningWindowThreshold {
		return SocketDegraded
	}
	return SocketHealthy
}
