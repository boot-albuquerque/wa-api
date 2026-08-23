// Package core owns the lifecycle of a headless session and is the composition
// root of this stack: it wires engine/, spa/, capabilities/ and runtime/
// together and holds the state that outlives a single command.
//
// A session here is a LIVE BROWSER, and that is the difference that shapes this
// package. In internal/wa-noise/ a session is a socket; resuming one was
// measured at 1.2-2.3s (ADR-0005). Here the spike measured 6.1-8.5s just to
// boot the SPA, pre-login. Every timeout, lease TTL and readiness rule in this
// package must be dimensioned from that number, not borrowed from the other
// stack — ADR-0006 D5.
//
// Ownership of a session follows ADR-0005 D2 unchanged in mechanism: one owner,
// heartbeat renews, TTL covers abrupt death. Only the numbers differ.
//
// Nothing outside this tree imports core/. Consumers use the facade in
// ../main.go.
package core
