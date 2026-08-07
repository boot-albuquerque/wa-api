package profile

import (
	"testing"

	whatsmeow "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/protocol/types"
)

// TestProfileDataAccess_NewProfileDataAccess cria adapter com client.
func TestProfileDataAccess_NewProfileDataAccess(t *testing.T) {
	da := NewProfileDataAccess(&whatsmeow.Client{})
	if da == nil {
		t.Fatal("NewProfileDataAccess returned nil")
	}
}

// TestProfileDataAccess_PushName_WithStore devolve Store.PushName.
func TestProfileDataAccess_PushName_WithStore(t *testing.T) {
	da := &ProfileDataAccess{client: &whatsmeow.Client{Store: &store.Device{PushName: "Alice"}}}
	if got := da.PushName(); got != "Alice" {
		t.Errorf("PushName = %q, want Alice", got)
	}
}

// TestProfileDataAccess_OwnJID_OK com Store.ID preenchido.
func TestProfileDataAccess_OwnJID_OK(t *testing.T) {
	myJID := types.NewJID("5511", types.DefaultUserServer)
	da := &ProfileDataAccess{client: &whatsmeow.Client{Store: &store.Device{ID: &myJID}}}
	got, ok := da.OwnJID()
	if !ok {
		t.Error("OwnJID returned !ok")
	}
	if got != "5511@s.whatsapp.net" {
		t.Errorf("OwnJID = %q, want 5511@s.whatsapp.net", got)
	}
}
