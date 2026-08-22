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
// The facade is still empty, but the tree is no longer: CAP-02 landed the
// foundation — deadline policy, runner, operation tracing, clean CDP shutdown
// and target priming, in engine/ and observability/. Nothing outside this tree
// consumes it yet, so there is nothing to re-export; the first alias belongs
// with the first capability that has a consumer.
//
// gate_test.go holds the module-wide rules, and it is the enforcement this
// comment used to only ask for: the driver stays inside engine/, no wait keeps
// its clock in the page, and priming never happens inside a bounded operation.
package waheadless

import "wa-api/internal/wa-headless/spa"

// The server suffixes this build files identities under.
//
// These are the facade's first symbols, and they are here for the reason the
// doc above states: the adapter in pkg/infra/wa-headless has to materialise
// THIS transport's canonical form, and the canonical form is not shared.
// Decision 74 measured why — the socket spells a phone identity
// s.whatsapp.net while this build spells it c.us, and the vendored socket
// types call c.us "legacy" even though it is what the page uses now. Each
// adapter owns its own suffix; only the vocabulary is shared.
//
// Re-exported rather than reachable: an adapter that imported spa/ directly
// would be reaching past the facade, which is the one thing this file exists
// to prevent.
const (
	ServerLID   = spa.ServerLID
	ServerPhone = spa.ServerPhone
	ServerGroup = spa.ServerGroup
)

// IsUnresolvedIdentity reports whether a jid names a PERSON in the phone
// namespace, which this build does not index people by. It is decision 66's
// single rule, and the adapter needs it to refuse explicitly instead of
// answering wrongly.
func IsUnresolvedIdentity(jid string) bool { return spa.IsUnresolvedIdentity(jid) }
