package misc

import (
	"context"
	"errors"
	"testing"

	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/wa-noise/client/testkit"
)

// fakeLIDStore implements store.LIDStore with configurable PN↔LID mappings.
// Mirrors the REAL GetLIDForPN/GetPNForLID contract from
// internal/wa-noise/persistence/store/sqlstore/lidmap.go:113-125.
type fakeLIDStore struct {
	store.NoopStore
	pnToLID map[string]types.JID
	lidToPN map[string]types.JID
}

func (f *fakeLIDStore) GetLIDForPN(_ context.Context, pn types.JID) (types.JID, error) {
	if pn.Server != types.DefaultUserServer {
		return types.JID{}, errors.New("not a PN")
	}
	if lid, ok := f.pnToLID[pn.String()]; ok {
		return lid, nil
	}
	return types.JID{}, nil
}

func (f *fakeLIDStore) GetPNForLID(_ context.Context, lid types.JID) (types.JID, error) {
	if lid.Server != types.HiddenUserServer {
		return types.JID{}, errors.New("not a LID")
	}
	if pn, ok := f.lidToPN[lid.String()]; ok {
		return pn, nil
	}
	return types.JID{}, nil
}

func (f *fakeLIDStore) PutManyLIDMappings(_ context.Context, _ []store.LIDMapping) error { return nil }
func (f *fakeLIDStore) PutLIDMapping(_ context.Context, _, _ types.JID) error            { return nil }
func (f *fakeLIDStore) GetManyLIDsForPNs(_ context.Context, _ []types.JID) (map[types.JID]types.JID, error) {
	return nil, nil
}

func storeWithLIDs(pnToLID map[string]types.JID, lidToPN map[string]types.JID) *store.Device {
	return &store.Device{
		LIDs: &fakeLIDStore{pnToLID: pnToLID, lidToPN: lidToPN},
	}
}

const (
	ownerPN  = "5516981818244@s.whatsapp.net"
	ownerLID = "29343770251463@lid"
	lucasLID = "90937376170214@lid"
)

// --- Test 1: PN payload is converted to LID before reaching the SDK ---

func TestDemoteNewsletterAdmin_PNConvertedToLID(t *testing.T) {
	var seenUser types.JID
	dev := storeWithLIDs(
		map[string]types.JID{ownerPN: mustParseJID(ownerLID)},
		map[string]types.JID{ownerLID: mustParseJID(ownerPN)},
	)
	fake := &testkit.Fake{
		StoreFn: func() *store.Device { return dev },
		NewsletterDemoteAdminFn: func(_ context.Context, _, userJID types.JID) error {
			seenUser = userJID
			return nil
		},
	}
	a := comCliente(fake)
	if err := a.DemoteNewsletterAdmin(context.Background(), "u1", canalJID, domain.JID(ownerPN)); err != nil {
		t.Fatalf("DemoteNewsletterAdmin = %v", err)
	}
	if seenUser.String() != ownerLID {
		t.Fatalf("user JID sent to SDK = %q, want %q (LID form)", seenUser.String(), ownerLID)
	}
}

func TestChangeNewsletterOwner_PNConvertedToLID(t *testing.T) {
	var seenOwner types.JID
	dev := storeWithLIDs(
		map[string]types.JID{ownerPN: mustParseJID(ownerLID)},
		map[string]types.JID{ownerLID: mustParseJID(ownerPN)},
	)
	fake := &testkit.Fake{
		StoreFn: func() *store.Device { return dev },
		NewsletterChangeOwnerFn: func(_ context.Context, _, newOwnerJID types.JID) error {
			seenOwner = newOwnerJID
			return nil
		},
	}
	a := comCliente(fake)
	if err := a.ChangeNewsletterOwner(context.Background(), "u1", canalJID, domain.JID(ownerPN)); err != nil {
		t.Fatalf("ChangeNewsletterOwner = %v", err)
	}
	if seenOwner.String() != ownerLID {
		t.Fatalf("owner JID sent to SDK = %q, want %q (LID form)", seenOwner.String(), ownerLID)
	}
}

// --- Test 2: LID payload stays LID (the directional guard) ---

func TestDemoteNewsletterAdmin_LIDStaysLID(t *testing.T) {
	var seenUser types.JID
	dev := storeWithLIDs(
		map[string]types.JID{ownerPN: mustParseJID(ownerLID)},
		map[string]types.JID{ownerLID: mustParseJID(ownerPN)},
	)
	fake := &testkit.Fake{
		StoreFn: func() *store.Device { return dev },
		NewsletterDemoteAdminFn: func(_ context.Context, _, userJID types.JID) error {
			seenUser = userJID
			return nil
		},
	}
	a := comCliente(fake)
	if err := a.DemoteNewsletterAdmin(context.Background(), "u1", canalJID, domain.JID(ownerLID)); err != nil {
		t.Fatalf("DemoteNewsletterAdmin = %v", err)
	}
	if seenUser.String() != ownerLID {
		t.Fatalf("LID was modified to %q, want %q — directional guard broken", seenUser.String(), ownerLID)
	}
}

// --- Control negative for Test 2: unconditional GetAltJID breaks LID ---

func TestDemoteNewsletterAdmin_ControlNegative_UnconditionalConversionBreaksLID(t *testing.T) {
	dev := storeWithLIDs(
		map[string]types.JID{ownerPN: mustParseJID(ownerLID)},
		map[string]types.JID{ownerLID: mustParseJID(ownerPN)},
	)
	lidJID := mustParseJID(ownerLID)

	alt, err := dev.GetAltJID(context.Background(), lidJID)
	if err != nil {
		t.Fatalf("GetAltJID: %v", err)
	}
	if alt.Server != types.DefaultUserServer {
		t.Fatalf("expected GetAltJID(LID) to return PN, got %q — the bidirectional behavior changed", alt.String())
	}
	if alt.String() == ownerLID {
		t.Errorf("GetAltJID(LID) returned LID unchanged — control negative invalid")
	}
}

// --- Test 3: no mapping available — keeps the payload, logs warning ---

func TestDemoteNewsletterAdmin_NoMappingKeepsPN(t *testing.T) {
	var seenUser types.JID
	dev := storeWithLIDs(nil, nil) // no mappings
	fake := &testkit.Fake{
		StoreFn: func() *store.Device { return dev },
		NewsletterDemoteAdminFn: func(_ context.Context, _, userJID types.JID) error {
			seenUser = userJID
			return nil
		},
	}
	a := comCliente(fake)
	if err := a.DemoteNewsletterAdmin(context.Background(), "u1", canalJID, domain.JID(ownerPN)); err != nil {
		t.Fatalf("DemoteNewsletterAdmin = %v", err)
	}
	if seenUser.String() != ownerPN {
		t.Fatalf("user JID = %q, want %q — with no mapping the original should be kept", seenUser.String(), ownerPN)
	}
}

// --- Test 3b: no store available — keeps the payload ---

func TestDemoteNewsletterAdmin_NoStoreKeepsPN(t *testing.T) {
	var seenUser types.JID
	fake := &testkit.Fake{
		NewsletterDemoteAdminFn: func(_ context.Context, _, userJID types.JID) error {
			seenUser = userJID
			return nil
		},
	}
	a := comCliente(fake)
	if err := a.DemoteNewsletterAdmin(context.Background(), "u1", canalJID, domain.JID(ownerPN)); err != nil {
		t.Fatalf("DemoteNewsletterAdmin = %v", err)
	}
	if seenUser.String() != ownerPN {
		t.Fatalf("user JID = %q, want %q — with no store the original should be kept", seenUser.String(), ownerPN)
	}
}

func mustParseJID(s string) types.JID {
	parts := splitJID(s)
	return types.NewJID(parts[0], parts[1])
}

func splitJID(s string) [2]string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '@' {
			return [2]string{s[:i], s[i+1:]}
		}
	}
	return [2]string{s, ""}
}
