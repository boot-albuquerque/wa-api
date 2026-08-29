// Package engine is the boundary between this stack and the browser.
//
// It exists so that ADR-0006 D1 (chromedp) is a decision recorded in ONE place
// instead of a dependency spread across the tree. Everything above this package
// talks to an interface; chromedp is imported here and nowhere else.
//
// That boundary is not ceremony. ADR-0006 D1 explicitly leaves the engine
// choice reversible: it was made on the grounds that chromedp does not decide
// silently for us, and D3 may force a re-evaluation if detection mitigation
// needs an ecosystem chromedp lacks. A decision that may be revisited must be
// isolated when it is made, not when it is revisited.
//
// The confinement is enforced, not asked for: ../gate_test.go fails the build
// if anything outside this directory imports the driver.
//
// This package also owns the launch flag set (ADR-0006 D2). Flags are named
// constants with the reason beside each one — never inherited from a library
// default. The spike measured a 790 MB -> 592 MB swing from three flags, so the
// flag set IS the memory budget, and changing it requires a new measurement.
//
// What lives here after CAP-02, and where each piece came from:
//
//	deadline.go  DeadlinePolicy — a deadline per operation class, never per
//	             call site (study phase 4C, sections 3-6)
//	runner.go    Runner.Do — the only sanctioned way to run a remote operation,
//	             and the only place that decides timeout vs application error
//	cdp.go       a raw browser-level CDP connection, because chromedp.Cancel
//	             was measured NOT putting Browser.close on the wire (4C §22)
//	shutdown.go  CleanStop — ask, WAIT for the exit, and only then signal.
//	             SIGTERM corrupts session state; the exit is the verdict, not
//	             the acknowledgement
//	prime.go     PrimeTab — materialise the target before any bounded operation
//
// Two static gates guard it: shutdown_policy_test.go (nothing stops a browser
// by signal outside the one marked fallback) and ../gate_test.go.
package engine
