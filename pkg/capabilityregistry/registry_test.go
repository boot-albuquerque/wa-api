package capabilityregistry

import (
	"testing"

	"wa-api/pkg/domain"
)

// TestDecide_KnownNotImplementedOnOneEngine_SupportedOnOther exercises a
// clear-cut pair: send_carousel is field-verified supported on wa_noise
// (HOUSEKEEP.md F216) and unknown on wa_headless (no adapter found).
func TestDecide_KnownNotImplementedOnOneEngine_SupportedOnOther(t *testing.T) {
	r := NewCapabilityRegistry()

	noise, err := r.Decide(domain.CapSendCarousel, domain.EngineNoise, domain.AccountTypeUnknown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !noise.Supported || noise.Status != domain.StatusSupported {
		t.Fatalf("send_carousel on wa_noise: got Supported=%v Status=%v, want Supported=true Status=supported", noise.Supported, noise.Status)
	}
	if noise.Evidence != domain.EvidenceConfirmed {
		t.Fatalf("send_carousel on wa_noise: got Evidence=%v, want confirmed (F216)", noise.Evidence)
	}

	headless, err := r.Decide(domain.CapSendCarousel, domain.EngineHeadless, domain.AccountTypeUnknown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headless.Supported {
		t.Fatalf("send_carousel on wa_headless: got Supported=true, want false (no adapter found)")
	}
	if headless.Status != domain.StatusUnknown {
		t.Fatalf("send_carousel on wa_headless: got Status=%v, want unknown", headless.Status)
	}
	if headless.Evidence != domain.EvidenceUnknown {
		t.Fatalf("send_carousel on wa_headless: got Evidence=%v, want unknown (grep found no adapter at all, not \"probable\" code that plausibly implements it)", headless.Evidence)
	}
}

// TestDecide_ConfirmedEngineUnsupported exercises the one row this pass
// could confirm as a documented, cited engine gap rather than a mere grep
// absence: SetGroupPhoto on wa_headless (H140).
func TestDecide_ConfirmedEngineUnsupported(t *testing.T) {
	r := NewCapabilityRegistry()

	d, err := r.Decide(domain.CapSetGroupPhoto, domain.EngineHeadless, domain.AccountTypeUnknown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Supported {
		t.Fatalf("set_group_photo on wa_headless: got Supported=true, want false (H140: page photo modules absent)")
	}
	if d.Status != domain.StatusEngineUnsupported {
		t.Fatalf("set_group_photo on wa_headless: got Status=%v, want engine_unsupported", d.Status)
	}
	if d.Evidence != domain.EvidenceConfirmed {
		t.Fatalf("set_group_photo on wa_headless: got Evidence=%v, want confirmed", d.Evidence)
	}
}

// TestDecide_NeverReturnsSupportedForAnUnknownCell is the registry's central
// safety property: an (capability, engine, account_type) combination with no
// matrix entry at all must come back unsupported+unknown, never quietly
// "supported" — the registry PROPAGATES uncertainty, it never masks it.
func TestDecide_NeverReturnsSupportedForAnUnknownCell(t *testing.T) {
	r := NewCapabilityRegistry()

	d, err := r.Decide(domain.Capability("totally_unregistered_capability"), domain.EngineNoise, domain.AccountTypeUnknown)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Supported {
		t.Fatalf("unregistered capability: got Supported=true, want false")
	}
	if d.Status != domain.StatusUnknown {
		t.Fatalf("unregistered capability: got Status=%v, want unknown", d.Status)
	}
	if d.Evidence != domain.EvidenceUnknown {
		t.Fatalf("unregistered capability: got Evidence=%v, want unknown", d.Evidence)
	}
}

// TestDecide_UnknownEngineIsAnError confirms Provider/Decide refuse an
// engine this registry does not know, rather than silently defaulting to
// one of the two real engines (the cross-engine-fallback ban, item 53).
func TestDecide_UnknownEngineIsAnError(t *testing.T) {
	r := NewCapabilityRegistry()

	_, err := r.Decide(domain.CapSendText, domain.Engine("made_up_engine"), domain.AccountTypeUnknown)
	if err == nil {
		t.Fatalf("expected an error for an unknown engine, got nil")
	}
}

// TestDecide_FailedPreconditionOverridesSupported confirms that a caller-
// supplied failed precondition downgrades an otherwise-supported decision to
// permission_required, and that Supported becomes false.
func TestDecide_FailedPreconditionOverridesSupported(t *testing.T) {
	r := NewCapabilityRegistry()

	d, err := r.Decide(domain.CapSetGroupLocked, domain.EngineNoise, domain.AccountTypeUnknown, "group_admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Supported {
		t.Fatalf("expected Supported=false once a precondition fails, got true")
	}
	if d.Status != domain.StatusPermissionRequired {
		t.Fatalf("got Status=%v, want permission_required", d.Status)
	}
	if len(d.FailedPreconditions) != 1 || d.FailedPreconditions[0] != "group_admin" {
		t.Fatalf("got FailedPreconditions=%v, want [group_admin]", d.FailedPreconditions)
	}
}

// TestProvider_CapabilitiesNonEmptyForBothEngines is a basic sanity check
// that the matrix actually populated both providers.
func TestProvider_CapabilitiesNonEmptyForBothEngines(t *testing.T) {
	r := NewCapabilityRegistry()

	for _, engine := range []domain.Engine{domain.EngineNoise, domain.EngineHeadless} {
		p, err := r.Provider(engine)
		if err != nil {
			t.Fatalf("Provider(%q): %v", engine, err)
		}
		if len(p.Capabilities()) == 0 {
			t.Fatalf("Provider(%q).Capabilities() is empty", engine)
		}
	}
}
