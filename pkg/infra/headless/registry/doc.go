// Package registry keeps track of which headless sessions this process holds,
// mirroring pkg/infra/noise/registry.
//
// It is the point where ADR-0005 D2 (ownership by lease) meets this stack: the
// registry is what the process consults to answer "do I own this session, and
// is it actually up". ADR-0005 D6 makes that two questions, not one — intent
// (`users.connected`) is not observed state, and a readiness probe that
// conflates them lies.
//
// A browser makes the gap wider than the socket stack does. A live Chrome with
// a dead SPA is a running process by every check the operating system can make,
// which is the same failure mode F89 measured for the socket stack. Liveness
// here has to be asked of the PAGE, not of the process.
package registry
