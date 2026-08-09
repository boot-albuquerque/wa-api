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
// This package also owns the launch flag set (ADR-0006 D2). Flags are named
// constants with the reason beside each one — never inherited from a library
// default. The spike measured a 790 MB -> 592 MB swing from three flags, so the
// flag set IS the memory budget, and changing it requires a new measurement.
package engine
