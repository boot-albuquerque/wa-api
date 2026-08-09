// Package runtime owns how many browsers exist and for how long: the instance
// pool, recycling policy and the capacity ceiling.
//
// This package exists because of one measurement. The spike put a pre-login
// session at 474-790 MB across 6 to 9 processes, scaling linearly to three
// concurrent sessions with no degradation. That makes memory, not CPU, the
// binding constraint of this stack, and it puts a 16 GB host in the range of
// TENS of sessions rather than hundreds.
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
