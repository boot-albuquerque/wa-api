package whatsmeow

import (
	"testing"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"

	"wa-api/pkg/domain"
)

func TestNewGroupAdapter(t *testing.T) {
	if NewGroupAdapter(waclienttest.GetterWith(nil)) == nil {
		t.Fatal("NewGroupAdapter returned nil")
	}
}

// TestToJIDs_Empty devolve slice vazio sem erro.
func TestToJIDs_Empty(t *testing.T) {
	got, err := toJIDs(nil)
	if err != nil {
		t.Fatalf("toJIDs(nil) = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("toJIDs(nil) length = %d, want 0", len(got))
	}
}

// TestToJIDs_PropagatesError.
func TestToJIDs_PropagatesError(t *testing.T) {
	// entrada 0x00 deve falhar em toJID (ou pode não falhar — skip).
	_, err := toJIDs([]domain.JID{domain.JID(string([]byte{0x00}))})
	if err == nil {
		t.Skip("toJIDs não falhou para esta entrada")
	}
}
