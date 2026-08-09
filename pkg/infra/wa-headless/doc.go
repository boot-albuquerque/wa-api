// Package waheadless is the application's side of the browser-automation
// stack: it adapts internal/wa-headless to what wa-api needs, exactly as
// pkg/infra/wa-noise adapts the protocol stack.
//
// The split is the same one that already works for the other stack:
//
//	internal/wa-headless/   the library — knows browsers and web.whatsapp.com,
//	                        knows nothing about wa-api
//	pkg/infra/wa-headless/  the adaptation — knows wa-api's ports, envelopes,
//	                        errors and logging, and speaks to the library only
//	                        through its facade
//
// Import rule: everything here imports wa-api/internal/wa-headless, the facade.
// Nothing here imports its subpackages. When a symbol is missing, the fix is to
// add the line to the facade, never to reach past it.
//
// The two stacks are alternative transports for the same product intent, so
// what lives here is expected to satisfy the SAME application ports that
// pkg/infra/wa-noise satisfies wherever a use case should not care which
// transport is behind it. Where a capability exists in only one of them, that
// asymmetry is a product decision and belongs in an ADR, not in a silent
// difference between two adapters.
package waheadless
