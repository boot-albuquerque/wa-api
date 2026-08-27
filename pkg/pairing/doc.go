// Package pairing routes the three pairing operations — QR, phone code and
// connect — to the provider of the engine the request NAMES, after confirming
// that engine is the one the TARGET session was created with.
//
// # The defect this package exists to remove (HOUSEKEEP F273)
//
// Until 2026-08-27 the handlers behind GET /session/qr, GET /session/connect
// and POST /session/pairphone were wired, in pkg/bootstrap/wiring_handlers.go,
// to a single hardcoded wa-noise adapter. A session created with
// engine=wa_headless persisted correctly, reported correctly on
// GET /session/capabilities, and then paired over the socket anyway, because
// nothing between the handler and the adapter ever read the persisted engine.
// The old selection mechanism (bootstrap.EngineSelection.EngineFor /
// UsaHeadless) was dead in production: grep found it referenced only by its own
// tests.
//
// # Why resolution is centralized here and not an `if` in each handler
//
// Three handlers times two engines is six branches, and each new engine or
// pairing method multiplies them. More importantly the branch is not the hard
// part — the ORDER is. Every request has to answer four questions, and each one
// has to be answered before any provider is touched:
//
//	1. is the named engine a real engine?          -> invalid_engine (400)
//	2. is it the TARGET session's engine?          -> engine_mismatch (409)
//	3. does that engine serve this capability?     -> capability_not_supported (422)
//	4. is a provider actually wired for it here?   -> engine_unavailable (409)
//
// A handler that got the order wrong would call a provider and only then
// discover the request should have been refused — which for pairing means
// touching a WhatsApp session on the wrong transport. Resolve is the single
// place that order is written down, and it is the reason no handler in this
// package's callers holds an engine name at all.
//
// # No cross-engine fallback, ever
//
// Question 3 answering "no" ends the request. There is deliberately no path
// from a wa_headless request to a wa-noise provider. Falling back would pair a
// session on a transport its record does not name, and the record is what every
// later operation on that session reads.
//
// # Actor and target are different questions
//
// Resolve takes the TARGET session id — the session being paired — and reads
// its engine from the repository. It never sees, and cannot consult, the actor:
// the token that authorized the call identifies WHO is asking, and on a
// self-service route the two happen to coincide. Coinciding is not the same as
// being the same question, and the day a route lets an operator pair someone
// else's session, reading the actor's engine would pair it on the wrong
// transport with every check reporting success.
package pairing
