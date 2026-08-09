// Package waheadless is the facade of the browser-automation stack: the only
// import path consumers outside this tree may use.
//
// The tree is layered, and the layering is the point:
//
//	core/           session lifecycle and composition root — implementation
//	engine/         the browser boundary; chromedp lives BEHIND this interface
//	spa/            what we know about web.whatsapp.com: hook discovery, injection
//	capabilities/   what the stack can do, one package per capability
//	runtime/        instance pool, recycling, capacity
//	observability/  logging bridge
//
// Dependency direction, declared: nothing outside this tree imports anything
// but this file. `core/` is implementation detail. The same rule in
// internal/wa-noise/ is enforced by scripts/waclient-facade-check.sh, and the
// lesson that produced that script applies here in advance — a rule that does
// not fail the build is a comment, not a rule.
//
// Writing rule: this file holds aliases and delegation only. Anything that
// needs a body lives in core/ or in capabilities/.
//
// The facade is empty today because the tree has no behavior yet: ADR-0006 is
// still `proposed`, and the skeleton exists to fix the SHAPE before the code
// arrives, not to pretend the code is there.
package waheadless
