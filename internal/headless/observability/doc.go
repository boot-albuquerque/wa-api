// Package observability bridges this stack's logging into the application's
// structured zerolog, the same role internal/wa-noise/observability/log plays
// for the protocol stack.
//
// It carries one requirement the other stack does not have: a browser writes to
// stderr on its own, in its own format, at a volume nobody chose. Whatever this
// package does with that output, it must not let it reach the application log
// unstructured — and it must not silently discard it either, because browser
// stderr is where a crash announces itself.
//
// F90 in the root HOUSEKEEP.md is the warning worth reading first: a client
// disconnect logged as `error` reads like a database failure. Severity is part
// of the contract, not decoration.
package observability
