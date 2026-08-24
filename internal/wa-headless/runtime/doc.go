// Package runtime owns how many browsers exist and for how long: the instance
// pool, recycling policy and the capacity ceiling.
//
// This package exists because of one measurement. The spike put a pre-login
// session at 474-790 MB across 6 to 9 processes, scaling linearly to three
// concurrent sessions with no degradation. That makes memory, not CPU, the
// binding constraint of this stack, and it puts a 16 GB host in the range of
// TENS of sessions rather than hundreds.
//
// RE-MEASURED 2026-08-20 (HOUSEKEEP H33), the first time this repository ran
// more than one browser at once. The SHAPE of that claim held and then some —
// per-session cost FALLS to 0.85x with three concurrent sessions, and no
// already-running session stopped answering when another arrived. The NUMBER
// did not: 1023 MB per pre-login session on this host, 30% above the inherited
// ceiling of 790 MB, with the same 9 processes. More memory in the same
// topology, not a different one.
//
// So the arithmetic above moves: ~15 sessions in 16 GB rather than ~20. Treat
// 474-790 MB as what it always was — a measurement from another moment, on
// another machine — and not as a constant. A PAIRED session costs more again:
// the retention run of the same day saw one settle at 700-800 MB after peaking
// at 1740 MB, and CONCURRENT paired sessions remain unmeasured, because the two
// paired profiles here belong to one account and running both would put two
// live devices on it.
//
// ADR-0006 D6 is explicit that those numbers are a FLOOR: they were measured
// pre-login, and a paired session with loaded history costs more by an amount
// nobody has measured yet. Any admission or capacity rule written here must
// treat the configured ceiling as an operator-supplied number validated against
// a real measurement, never as a constant derived from the spike.
//
// Degradation is loud, never silent — the same rule ADR-0005 D7 sets for the
// process as a whole. Refusing a session because the host is full is a decision
// to report, not a wait to hide.
package runtime
