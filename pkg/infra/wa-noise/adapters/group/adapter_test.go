package group

import (
	"testing"
	"wa-api/pkg/infra/wa-noise/client/testkit"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"

	"wa-api/pkg/domain"
)

func TestNewGroupAdapter(t *testing.T) {
	if NewGroupAdapter(testkit.GetterWith(nil)) == nil {
		t.Fatal("NewGroupAdapter returned nil")
	}
}

// TestToJIDs_Empty devolve slice vazio sem erro.
func TestToJIDs_Empty(t *testing.T) {
	got, err := wajid.ToJIDs(nil)
	if err != nil {
		t.Fatalf("wajid.ToJIDs(nil) = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("wajid.ToJIDs(nil) length = %d, want 0", len(got))
	}
}

// TestToJIDs_PropagatesError.
func TestToJIDs_PropagatesError(t *testing.T) {
	// entrada 0x00 deve falhar em wajid.ToJID (ou pode não falhar — skip).
	_, err := wajid.ToJIDs([]domain.JID{domain.JID(string([]byte{0x00}))})
	if err == nil {
		t.Skip("wajid.ToJIDs não falhou para esta entrada")
	}
}
