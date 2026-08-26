package domain

import "testing"

func TestNewCanonicalAccountIdentity_PN(t *testing.T) {
	id, ok := NewCanonicalAccountIdentity(JID("5511999999999" + ServerPN))
	if !ok {
		t.Fatal("expected ok for a PN JID")
	}
	if !id.IsResolved() {
		t.Error("a PN identity must already be resolved")
	}
	if got, want := id.String(), ServerPN+":5511999999999"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestNewCanonicalAccountIdentity_LID_NotResolved(t *testing.T) {
	id, ok := NewCanonicalAccountIdentity(JID("123456789" + ServerLID))
	if !ok {
		t.Fatal("expected ok for a LID JID")
	}
	if id.IsResolved() {
		t.Error("a bare LID identity must NOT be reported as resolved: nothing invented a PN for it")
	}
}

func TestNewCanonicalAccountIdentity_RejectsOtherServers(t *testing.T) {
	for _, jid := range []JID{
		"group-id@g.us",
		"channel@newsletter",
		"status@broadcast",
		"",
	} {
		if _, ok := NewCanonicalAccountIdentity(jid); ok {
			t.Errorf("NewCanonicalAccountIdentity(%q): expected ok=false, got true", jid)
		}
	}
}

// TestDeviceSuffixDoesNotChangeIdentity is the property the whole type exists
// for: two devices of the same account must canonicalize to the SAME
// identity, or the exclusivity key would treat one person as many.
func TestDeviceSuffixDoesNotChangeIdentity(t *testing.T) {
	a, ok := NewCanonicalAccountIdentity(JID("5511999999999" + ServerPN))
	if !ok {
		t.Fatal("setup: expected ok")
	}
	b, ok := NewCanonicalAccountIdentity(JID("5511999999999:12" + ServerPN))
	if !ok {
		t.Fatal("setup: expected ok")
	}
	if a.String() != b.String() {
		t.Errorf("device suffix changed the canonical identity: %q vs %q", a.String(), b.String())
	}
}

// TestPNAndLIDAreDistinctIdentities: without an explicit resolution, a PN and
// a LID for the (possibly) same real person must NOT collapse into one key.
// Collapsing them without proof would let an attacker-controlled LID steal
// ownership of a PN-known account, or vice versa.
func TestPNAndLIDAreDistinctIdentities(t *testing.T) {
	pn, _ := NewCanonicalAccountIdentity(JID("5511999999999" + ServerPN))
	lid, _ := NewCanonicalAccountIdentity(JID("5511999999999" + ServerLID))
	if pn.String() == lid.String() {
		t.Fatal("PN and LID identities must be distinct until explicitly resolved")
	}
}

func TestWithResolvedPN(t *testing.T) {
	lid, _ := NewCanonicalAccountIdentity(JID("123456789" + ServerLID))
	resolved, ok := lid.WithResolvedPN(JID("5511999999999" + ServerPN))
	if !ok {
		t.Fatal("expected WithResolvedPN to accept a valid PN")
	}
	if !resolved.IsResolved() {
		t.Error("identity resolved via WithResolvedPN must report IsResolved() == true")
	}
	if got, want := resolved.String(), ServerPN+":5511999999999"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	// The original value is untouched — WithResolvedPN returns a copy.
	if lid.IsResolved() {
		t.Error("WithResolvedPN mutated the receiver; it must return a copy")
	}
}

func TestWithResolvedPN_RejectsNonPN(t *testing.T) {
	lid, _ := NewCanonicalAccountIdentity(JID("123456789" + ServerLID))
	if _, ok := lid.WithResolvedPN(JID("other" + ServerLID)); ok {
		t.Error("WithResolvedPN must reject a non-PN argument")
	}
}

func TestCanonicalAccountIdentity_IsZero(t *testing.T) {
	var zero CanonicalAccountIdentity
	if !zero.IsZero() {
		t.Error("zero value must report IsZero() == true")
	}
	id, _ := NewCanonicalAccountIdentity(JID("5511999999999" + ServerPN))
	if id.IsZero() {
		t.Error("a derived identity must not report IsZero()")
	}
}
