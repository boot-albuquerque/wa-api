package capabilityregistry

import (
	"testing"

	"wa-api/pkg/domain"
)

// This file characterizes the Engine × AccountType quadrant (NP/NB/HP/HB —
// wa_noise×personal, wa_noise×business, wa_headless×personal,
// wa_headless×business) for the subset of capabilities where the matrix in
// matrix.go already records a real difference between at least two of the
// four combinations. It is a regression lock, not new evidence: every
// assertion here is read directly off matrix.go, cited by row.
//
// NOTE ON ACCOUNT TYPE: as of this pass, domain.AccountType has no adapter
// that varies behavior by personal vs business (see CLAUDE.md task brief and
// matrix.go's expand doc comment). Every row below therefore has IDENTICAL
// status for personal and business on a given engine — matrix.expand()
// copies the unknown-account-type cell to both, only downgrading evidence to
// EvidenceNotTested. Where a test below asserts NP==NB (or HP==HB), that is
// confirming the CURRENT LIMITATION (account_type does not change the
// decision), not a claim that it never will.

// quadrant is the four decisions for one capability, named the way the task
// brief names them: N/H for engine (wa_noise/wa_headless), P/B for account
// type (personal/business).
type quadrant struct {
	np, nb, hp, hb CapabilityDecision
}

func decideQuadrant(t *testing.T, r *CapabilityRegistry, capability domain.Capability) quadrant {
	t.Helper()
	np, err := r.Decide(capability, domain.EngineNoise, domain.AccountTypePersonal)
	if err != nil {
		t.Fatalf("Decide(%q, wa_noise, personal): %v", capability, err)
	}
	nb, err := r.Decide(capability, domain.EngineNoise, domain.AccountTypeBusiness)
	if err != nil {
		t.Fatalf("Decide(%q, wa_noise, business): %v", capability, err)
	}
	hp, err := r.Decide(capability, domain.EngineHeadless, domain.AccountTypePersonal)
	if err != nil {
		t.Fatalf("Decide(%q, wa_headless, personal): %v", capability, err)
	}
	hb, err := r.Decide(capability, domain.EngineHeadless, domain.AccountTypeBusiness)
	if err != nil {
		t.Fatalf("Decide(%q, wa_headless, business): %v", capability, err)
	}
	return quadrant{np, nb, hp, hb}
}

// TestQuadrant_SendCarousel: wa_noise supported+confirmed (HOUSEKEEP F216,
// device-photographed), wa_headless unknown (no adapter found at all).
// Pattern: NP=NB (supported) vs HP=HB (unknown) — an engine gap, not an
// account-type gap; account_type carries zero information here.
func TestQuadrant_SendCarousel(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapSendCarousel)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb} {
		if !d.Supported || d.Status != domain.StatusSupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want supported (evidence downgraded to not_tested per account type, see matrix.expand)", name, d.Supported, d.Status, d.Evidence)
		}
	}
	for name, d := range map[string]CapabilityDecision{"HP": q.hp, "HB": q.hb} {
		if d.Supported || d.Status != domain.StatusUnknown || d.Evidence != domain.EvidenceUnknown {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want unsupported/unknown", name, d.Supported, d.Status, d.Evidence)
		}
	}
}

// TestQuadrant_SetGroupPhoto: the one row with a CONFIRMED engine_unsupported
// cell (H140, cited in pkg/application/contracts/group_ports.go). wa_noise
// supported, wa_headless confirmed engine_unsupported on both account types.
// Pattern: NP=NB (supported) vs HP=HB (engine_unsupported) — engine gap,
// confirmed rather than merely absent-adapter.
func TestQuadrant_SetGroupPhoto(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapSetGroupPhoto)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb} {
		if !d.Supported || d.Status != domain.StatusSupported {
			t.Fatalf("%s: got Supported=%v Status=%v, want supported", name, d.Supported, d.Status)
		}
	}
	for name, d := range map[string]CapabilityDecision{"HP": q.hp, "HB": q.hb} {
		if d.Supported || d.Status != domain.StatusEngineUnsupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want engine_unsupported (evidence downgraded to not_tested per account type; unknown-type evidence was confirmed via H140)", name, d.Supported, d.Status, d.Evidence)
		}
	}
}

// TestQuadrant_SendText: the representative "no headless adapter written for
// message creation" row — wa-headless/messenger only covers actions on
// EXISTING messages (see matrix.go's messaging-section comment), so every
// "new message" capability including send_text is unknown there, not
// confirmed absent. Pattern: NP=NB (supported/probable) vs HP=HB
// (unknown/unknown).
func TestQuadrant_SendText(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapSendText)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb} {
		if !d.Supported || d.Status != domain.StatusSupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want supported (not_tested per account type)", name, d.Supported, d.Status, d.Evidence)
		}
	}
	for name, d := range map[string]CapabilityDecision{"HP": q.hp, "HB": q.hb} {
		if d.Supported || d.Status != domain.StatusUnknown || d.Evidence != domain.EvidenceUnknown {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want unknown/unknown", name, d.Supported, d.Status, d.Evidence)
		}
	}
}

// TestQuadrant_StarMessage: wa_noise supported via app-state patch
// (appstate.BuildStar), explicitly contradicting a stale claim in
// docs/CAPACIDADES.md (self-flagged desatualizado since 2026-08-24 per the
// row's note); wa_headless unknown (no adapter). Recorded separately from
// send_text/send_carousel because the "supported" side has a documented
// correction attached, which is exactly the kind of fact a regression test
// should pin so nobody silently reverts to the stale belief.
func TestQuadrant_StarMessage(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapStarMessage)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb} {
		if !d.Supported || d.Status != domain.StatusSupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want supported (not_tested per account type)", name, d.Supported, d.Status, d.Evidence)
		}
	}
	for name, d := range map[string]CapabilityDecision{"HP": q.hp, "HB": q.hb} {
		if d.Supported || d.Status != domain.StatusUnknown {
			t.Fatalf("%s: got Supported=%v Status=%v, want unsupported/unknown", name, d.Supported, d.Status)
		}
	}
}

// TestQuadrant_MarkRead: a rare SAME-status-on-both-engines row (supported on
// wa_noise AND wa_headless), used here to lock the opposite pattern from the
// engine-gap tests above: NP=NB=HP=HB (all supported/probable). The
// wa_headless side is "local ack only, receipt-to-sender NOT confirmed"
// (H160, cited in messenger.go) — Supported=true still holds because the
// registry's Supported reflects "the operation is served", not every
// observable side effect; this test locks that nuance too, not just the
// boolean.
func TestQuadrant_MarkRead(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapMarkRead)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb, "HP": q.hp, "HB": q.hb} {
		if !d.Supported || d.Status != domain.StatusSupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want supported on both engines (not_tested per account type)", name, d.Supported, d.Status, d.Evidence)
		}
	}
}

// TestQuadrant_DetectAccountType: the one row where the two engines have the
// SAME status (supported on both) but DIFFERENT evidence — wa_noise is
// EvidenceConfirmed (unit-tested against a real usync certificate fixture),
// wa_headless is EvidenceProbable (reuses a getter measured live on the lab
// account per HOUSEKEEP, but not re-confirmed by this worktree). This is the
// row most relevant to items 76-79 (engine × account_type interaction): even
// though this capability's whole PURPOSE is to determine account type, the
// matrix does not vary Status by AccountType — it cannot, since account type
// isn't known before this capability runs. Documented here as "does not
// apply" rather than silently skipped.
func TestQuadrant_DetectAccountType(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapDetectAccountType)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb} {
		if !d.Supported || d.Status != domain.StatusSupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want supported on wa_noise (not_tested per account type; unknown-type evidence was confirmed)", name, d.Supported, d.Status, d.Evidence)
		}
	}
	for name, d := range map[string]CapabilityDecision{"HP": q.hp, "HB": q.hb} {
		if !d.Supported || d.Status != domain.StatusSupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want supported on wa_headless (not_tested per account type; unknown-type evidence was probable)", name, d.Supported, d.Status, d.Evidence)
		}
	}
}

// TestQuadrant_CheckPairingStatus: wa_noise supported, wa_headless unknown —
// with a note explicitly flagging that engine_unsupported is PLAUSIBLE
// (headless auth is QR/cookie based, not pairing-code based) but NOT
// confirmed by reading the auth flow. Distinguished from
// TestQuadrant_SetGroupPhoto on purpose: same shape of "wa_noise yes,
// wa_headless no" but this one must stay Unknown, not EngineUnsupported,
// until someone actually reads the headless auth flow. If a future change
// promotes this cell to EngineUnsupported without that verification, this
// test is the tripwire.
func TestQuadrant_CheckPairingStatus(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapCheckPairingStatus)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb} {
		if !d.Supported || d.Status != domain.StatusSupported {
			t.Fatalf("%s: got Supported=%v Status=%v, want supported", name, d.Supported, d.Status)
		}
	}
	for name, d := range map[string]CapabilityDecision{"HP": q.hp, "HB": q.hb} {
		if d.Supported || d.Status != domain.StatusUnknown {
			t.Fatalf("%s: got Supported=%v Status=%v, want unsupported/unknown (NOT engine_unsupported — not yet confirmed)", name, d.Supported, d.Status)
		}
	}
}

// TestQuadrant_UpdateGroupParticipants: supported on both engines, but the
// wa_headless side carries a documented asymmetry (H58/H65: the write can
// succeed while the acting session cannot read it back). Status/Supported
// are identical across all four cells, so this test locks that the matrix
// does NOT (yet) encode that asymmetry as a status difference — it lives
// only in the note. That is itself worth pinning: a future change that
// downgrades wa_headless to reflect the read-back gap would need to update
// this test deliberately, not by accident.
func TestQuadrant_UpdateGroupParticipants(t *testing.T) {
	r := NewCapabilityRegistry()
	q := decideQuadrant(t, r, domain.CapUpdateGroupParticipants)

	for name, d := range map[string]CapabilityDecision{"NP": q.np, "NB": q.nb, "HP": q.hp, "HB": q.hb} {
		if !d.Supported || d.Status != domain.StatusSupported || d.Evidence != domain.EvidenceNotTested {
			t.Fatalf("%s: got Supported=%v Status=%v Evidence=%v, want supported on both engines (read-back asymmetry is note-only, not status-encoded)", name, d.Supported, d.Status, d.Evidence)
		}
	}
}

// TestQuadrant_AccountTypeNeverChangesStatusToday is the cross-cutting
// characterization for items 76-79: for every capability in the matrix,
// confirm personal and business get the IDENTICAL status on a given engine.
// This documents the current limitation from CLAUDE.md's task brief
// (account_type is not measured by real evidence anywhere yet) as an
// executable fact instead of prose that can go stale. If this test ever
// fails, it means a capability now DOES vary by account_type — which would
// be exactly the kind of real Engine×AccountType interaction items 76-79
// ask to classify, and this test failing is the signal to go write that
// classification.
func TestQuadrant_AccountTypeNeverChangesStatusToday(t *testing.T) {
	r := NewCapabilityRegistry()

	sample := []domain.Capability{
		domain.CapSendCarousel,
		domain.CapSetGroupPhoto,
		domain.CapSendText,
		domain.CapStarMessage,
		domain.CapMarkRead,
		domain.CapDetectAccountType,
		domain.CapCheckPairingStatus,
		domain.CapUpdateGroupParticipants,
		domain.CapMuteChat,
		domain.CapSetGroupEphemeralTimer,
	}

	for _, capability := range sample {
		q := decideQuadrant(t, r, capability)
		if q.np.Status != q.nb.Status || q.np.Supported != q.nb.Supported {
			t.Fatalf("%s: personal/business diverge on wa_noise (NP=%v/%v NB=%v/%v) — account_type now carries signal; update items 76-79 classification instead of this test", capability, q.np.Status, q.np.Supported, q.nb.Status, q.nb.Supported)
		}
		if q.hp.Status != q.hb.Status || q.hp.Supported != q.hb.Supported {
			t.Fatalf("%s: personal/business diverge on wa_headless (HP=%v/%v HB=%v/%v) — account_type now carries signal; update items 76-79 classification instead of this test", capability, q.hp.Status, q.hp.Supported, q.hb.Status, q.hb.Supported)
		}
	}
}
