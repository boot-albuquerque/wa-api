// Package spa holds everything this stack knows about web.whatsapp.com itself:
// the module inventory, injection, and what the page exposes.
//
// It is separated from engine/ because the two change for different reasons.
// engine/ changes when we change how we drive a browser; spa/ changes when Meta
// ships a new build. Putting them together would mean every WhatsApp-side
// change touching the code that launches Chrome.
//
// The integration surface is window.require('<ModuleName>') — verified against
// wwebjs main @ 942d236a11ad (2026-07-27), which makes 50 such calls in
// Client.js alone and no longer uses moduleRaid or the webpack chunk array.
// Those module names are undocumented Meta contract.
//
// ADR-0006 D4 governs this package, and it has one non-obvious rule: the
// inventory is verified AT SESSION START, failing loudly with the list of
// missing names. A rename then breaks in a predictable place with a message
// that names the cause, instead of surfacing as an exception halfway through a
// dispatch. Module names are named constants here and nowhere else; a literal
// at a call site is the defect this package exists to prevent.
//
// Anything learned here that diverges from how wwebjs does it belongs in
// ../PATCHES.md, anchored to the wwebjs file and commit it diverged from.
package spa
