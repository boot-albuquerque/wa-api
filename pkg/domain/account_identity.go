package domain

import "strings"

// CanonicalAccountIdentity normalizes a JID into the identity used to decide
// account exclusivity.
//
// # Why this is a NEW type and not an expansion of JID
//
// JID (jid.go) is a wire-format value: it carries a device suffix
// (":device") and distinguishes PN from LID only by server suffix. Two JIDs
// for the SAME WhatsApp account routinely differ as strings — one device
// vs. another, PN vs. LID for a contact whose LID has since been resolved —
// and comparing them with `==` would treat the same account as two.
// CanonicalAccountIdentity is the value that answers "is this the same
// account", which JID was never designed to answer. It is DERIVED from a
// JID (NewCanonicalAccountIdentity), never constructed independently, so
// there is exactly one place that encodes the normalization rule.
//
// # Resolution strategy: prefer PN, never invent it
//
// A WhatsApp account has one durable identity (the phone number, PN) and one
// privacy-preserving alias (LID) that the protocol increasingly prefers on
// the wire. Two JIDs for the same person can arrive as PN today and LID
// tomorrow. This type's rule:
//
//   - If the JID is PN, the canonical identity IS the PN (device suffix
//     stripped). Nothing to resolve.
//   - If the JID is LID and no PN is known, the canonical identity is the
//     LID (device suffix stripped) tagged as unresolved. It is NOT upgraded
//     to a guessed PN — inventing one would let two different real accounts
//     collide on a wrong guess, which is worse than not deduplicating them.
//   - When a caller later learns a reliable PN for a LID (from the noise
//     store, USync, or an event that ties the two), it calls
//     WithResolvedPN to update the identity. The zero-value rule ("do not
//     invent") only governs how the identity is CREATED; resolution is an
//     explicit, separate step so it can be audited.
type CanonicalAccountIdentity struct {
	// server is ServerPN or ServerLID: which space `value` lives in.
	server string
	// value is the user part of the JID (before '@'), with any ":device"
	// suffix stripped.
	value string
	// resolved is true only when server == ServerPN, OR when a caller
	// explicitly resolved a LID to its PN via WithResolvedPN.
	resolved bool
}

// NewCanonicalAccountIdentity derives the canonical identity from a JID.
//
// Returns ok=false for a JID that is neither PN nor LID (group, newsletter,
// broadcast, ...): those have no per-account identity to canonicalize, and a
// caller asking for one from such a JID has the wrong input, not a value
// this type should paper over.
func NewCanonicalAccountIdentity(j JID) (CanonicalAccountIdentity, bool) {
	switch {
	case j.IsPN():
		return CanonicalAccountIdentity{
			server:   ServerPN,
			value:    stripDevice(strings.TrimSuffix(string(j), ServerPN)),
			resolved: true,
		}, true
	case j.IsLID():
		return CanonicalAccountIdentity{
			server:   ServerLID,
			value:    stripDevice(strings.TrimSuffix(string(j), ServerLID)),
			resolved: false,
		}, true
	default:
		return CanonicalAccountIdentity{}, false
	}
}

// stripDevice removes the ":device" suffix noise appends to a JID's user
// part (e.g. "5511999999999:12" -> "5511999999999"). Two JIDs that differ
// only by device are the SAME account — the device index names which of a
// person's linked devices sent a message, not which person it is.
func stripDevice(userPart string) string {
	if i := strings.IndexByte(userPart, ':'); i >= 0 {
		return userPart[:i]
	}
	return userPart
}

// WithResolvedPN returns a copy of this identity upgraded to the given PN.
//
// It is a no-op-returning-copy (not a mutation) so the caller who resolved
// the PN decides, explicitly, whether to keep using the old value —
// resolution changes the canonical key an account claims ownership under,
// so silently mutating a value already handed to a claim in flight would be
// exactly the kind of hidden state change ARMADILHAS.md warns about.
//
// pn must itself be a PN JID; a non-PN input is rejected rather than
// silently ignored.
func (c CanonicalAccountIdentity) WithResolvedPN(pn JID) (CanonicalAccountIdentity, bool) {
	if !pn.IsPN() {
		return c, false
	}
	return CanonicalAccountIdentity{
		server:   ServerPN,
		value:    stripDevice(strings.TrimSuffix(string(pn), ServerPN)),
		resolved: true,
	}, true
}

// IsResolved reports whether this identity is anchored to a known PN, as
// opposed to a LID with no PN known yet.
func (c CanonicalAccountIdentity) IsResolved() bool { return c.resolved }

// String is the stable, comparable, storable form of the identity: the
// server tag plus the normalized user part. It is what gets written into the
// ownership table's key — see pkg/infra/db/account_ownership.go — so its
// format is part of the storage contract, not merely a debug aid.
func (c CanonicalAccountIdentity) String() string {
	if c.server == "" {
		return ""
	}
	return c.server + ":" + c.value
}

// IsZero reports whether this identity was never derived from a JID.
func (c CanonicalAccountIdentity) IsZero() bool { return c.server == "" }
