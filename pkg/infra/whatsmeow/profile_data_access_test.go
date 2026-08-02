package whatsmeow

import (
	"context"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

// TestProfileDataAccess_PushName_NilStore verifica que PushName retorna ""
// quando o Store é nil, sem panic.
func TestProfileDataAccess_PushName_NilStore(t *testing.T) {
	da := &ProfileDataAccess{client: &whatsmeow.Client{}}
	if got := da.PushName(); got != "" {
		t.Errorf("expected empty PushName with nil Store, got %q", got)
	}
}

// TestProfileDataAccess_OwnJID_NilStore verifica que OwnJID retorna
// ("", false) quando o Store é nil.
func TestProfileDataAccess_OwnJID_NilStore(t *testing.T) {
	da := &ProfileDataAccess{client: &whatsmeow.Client{}}
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
	da := &ProfileDataAccess{client: &whatsmeow.Client{}}
	_, ok := da.OwnJID()
	if ok {
		t.Error("expected ok=false with nil Store.ID")
	}
}

// TestProfileDataAccess_ContactInfo_NilContacts verifica que ContactInfo
// retorna ("", "", nil) quando Store.Contacts é nil.
func TestProfileDataAccess_ContactInfo_NilContacts(t *testing.T) {
	da := &ProfileDataAccess{client: &whatsmeow.Client{}}
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
	cs := &fakeContactStore{contacts: map[types.JID]types.ContactInfo{}}
	da := &ProfileDataAccess{client: &whatsmeow.Client{Store: &store.Device{Contacts: cs}}}
	fullName, businessName, err := da.ContactInfo(context.Background(), "5511900000000@s.whatsapp.net")
	if err != nil {
		t.Errorf("expected nil error for missing contact, got %v", err)
	}
	if fullName != "" || businessName != "" {
		t.Errorf("expected empty names for missing contact, got %q / %q", fullName, businessName)
	}
}

// TestProfileDataAccess_ContactInfo_OK mapeia nomes quando o contato existe.
func TestProfileDataAccess_ContactInfo_OK(t *testing.T) {
	cs := &fakeContactStore{contacts: map[types.JID]types.ContactInfo{
		types.NewJID("5511987654321", types.DefaultUserServer): {FullName: "Alice", BusinessName: "Acme"},
	}}
	da := &ProfileDataAccess{client: &whatsmeow.Client{Store: &store.Device{Contacts: cs}}}
	fullName, businessName, err := da.ContactInfo(context.Background(), "5511987654321@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ContactInfo = %v", err)
	}
	if fullName != "Alice" || businessName != "Acme" {
		t.Errorf("ContactInfo = (%q, %q), want (Alice, Acme)", fullName, businessName)
	}
}

// TestProfileDataAccess_ContactInfo_PropagatesError.
func TestProfileDataAccess_ContactInfo_PropagatesError(t *testing.T) {
	cs := &fakeContactStore{errOnGet: errors.New("contacts fail")}
	da := &ProfileDataAccess{client: &whatsmeow.Client{Store: &store.Device{Contacts: cs}}}
	_, _, err := da.ContactInfo(context.Background(), "5511987654321@s.whatsapp.net")
	if err == nil {
		t.Fatal("ContactInfo não propagou erro")
	}
}

// TestProfileDataAccess_ProfilePictureURL_NilClient: cobre o caso de nil client.
func TestProfileDataAccess_ProfilePictureURL_NilClient(t *testing.T) {
	da := &ProfileDataAccess{client: nil}
	url, id, err := da.ProfilePictureURL(context.Background(), "x@s.whatsapp.net")
	if err != nil {
		t.Fatalf("ProfilePictureURL = %v", err)
	}
	if url != "" || id != "" {
		t.Errorf("ProfilePictureURL = (%q, %q)", url, id)
	}
}

// TestNewProfileDataAccessFromInterface_NilClient devolve adapter com client nil.
func TestNewProfileDataAccessFromInterface_NilClient(t *testing.T) {
	da := NewProfileDataAccessFromInterface(nil)
	if da == nil {
		t.Fatal("NewProfileDataAccessFromInterface(nil) returned nil")
	}
	if da.client != nil {
		t.Error("expected nil client")
	}
}

// TestNewProfileDataAccessFromInterface_RealClient desembrulva o cliente.
func TestNewProfileDataAccessFromInterface_RealClient(t *testing.T) {
	wac := &whatsmeow.Client{}
	rc := realWAClient{Client: wac}
	da := NewProfileDataAccessFromInterface(rc)
	if da.client != wac {
		t.Error("expected unwrapped *whatsmeow.Client")
	}
}

// TestNewProfileDataAccessFromInterface_FakeNotRealClient devolve com client nil.
func TestNewProfileDataAccessFromInterface_FakeNotRealClient(t *testing.T) {
	fake := &fakeWAClient{}
	da := NewProfileDataAccessFromInterface(fake)
	if da.client != nil {
		t.Error("expected nil client when interface is not realWAClient")
	}
}