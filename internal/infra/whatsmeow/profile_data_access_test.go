package whatsmeow

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow"
)

// TestProfileDataAccess_PushName_NilStore verifica que PushName retorna ""
// quando o Store é nil, sem panic.
func TestProfileDataAccess_PushName_NilStore(t *testing.T) {
	da := &ProfileDataAccess{
		client: &whatsmeow.Client{},
	}
	if got := da.PushName(); got != "" {
		t.Errorf("expected empty PushName with nil Store, got %q", got)
	}
}

// TestProfileDataAccess_OwnJID_NilStore verifica que OwnJID retorna
// ("", false) quando o Store é nil.
func TestProfileDataAccess_OwnJID_NilStore(t *testing.T) {
	da := &ProfileDataAccess{
		client: &whatsmeow.Client{},
	}
	jid, ok := da.OwnJID()
	if ok {
		t.Error("expected ok=false with nil Store")
	}
	if jid != "" {
		t.Errorf("expected empty JID, got %q", jid)
	}
}

// TestProfileDataAccess_OwnJID_NilID verifica que OwnJID retorna
// ("", false) quando Store.ID é nil.
func TestProfileDataAccess_OwnJID_NilID(t *testing.T) {
	da := &ProfileDataAccess{
		client: &whatsmeow.Client{},
	}
	_, ok := da.OwnJID()
	if ok {
		t.Error("expected ok=false with nil Store.ID")
	}
}

// TestProfileDataAccess_ContactInfo_NilContacts verifica que ContactInfo
// retorna ("", "", nil) quando Store.Contacts é nil.
func TestProfileDataAccess_ContactInfo_NilContacts(t *testing.T) {
	da := &ProfileDataAccess{
		client: &whatsmeow.Client{},
	}
	fullName, businessName, err := da.ContactInfo(context.Background(), "5511987654321@s.whatsapp.net")
	if err != nil {
		t.Errorf("expected nil error with nil contacts, got %v", err)
	}
	if fullName != "" || businessName != "" {
		t.Errorf("expected empty names, got %q / %q", fullName, businessName)
	}
}

// TestProfileDataAccess_ContactInfo_NoSuchContact verifica que ContactInfo
// retorna strings vazias quando o JID não está nos contatos.
func TestProfileDataAccess_ContactInfo_NoSuchContact(t *testing.T) {
	da := &ProfileDataAccess{
		client: &whatsmeow.Client{},
	}
	fullName, businessName, err := da.ContactInfo(context.Background(), "5511900000000@s.whatsapp.net")
	if err != nil {
		t.Errorf("expected nil error for missing contact, got %v", err)
	}
	if fullName != "" || businessName != "" {
		t.Errorf("expected empty names for missing contact, got %q / %q", fullName, businessName)
	}
}
