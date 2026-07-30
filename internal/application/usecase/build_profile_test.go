package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appport "wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// mockProfileData implementa port.ProfileDataAccess para testes internos.
type mockProfileData struct {
	pushName     string
	jid          string
	hasJID       bool
	avatarURL    string
	avatarID     string
	avatarErr    error
	fullName     string
	businessName string
	contactErr   error
}

var _ appport.ProfileDataAccess = (*mockProfileData)(nil)

func (m *mockProfileData) PushName() string { return m.pushName }

func (m *mockProfileData) OwnJID() (domain.JID, bool) {
	if !m.hasJID {
		return "", false
	}
	return domain.JID(m.jid), true
}

func (m *mockProfileData) ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error) {
	return m.avatarURL, m.avatarID, m.avatarErr
}

func (m *mockProfileData) ContactInfo(ctx context.Context, jid domain.JID) (string, string, error) {
	return m.fullName, m.businessName, m.contactErr
}

func TestBuildProfile_Complete(t *testing.T) {
	data := &mockProfileData{
		pushName:     "John Doe",
		jid:          "5511987654321@s.whatsapp.net",
		hasJID:       true,
		avatarURL:    "https://example.com/avatar.jpg",
		avatarID:     "av-abc123",
		fullName:     "John Full",
		businessName: "John's Business",
	}

	result := buildProfile(context.Background(), data)

	check := func(got, want string) {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	check(result.Pushname, "John Doe")
	check(result.JID, "5511987654321@s.whatsapp.net")
	check(result.AvatarURL, "https://example.com/avatar.jpg")
	check(result.AvatarID, "av-abc123")
	check(result.FullName, "John Full")
	check(result.BusinessName, "John's Business")
}

func TestBuildProfile_NoJID(t *testing.T) {
	data := &mockProfileData{pushName: "Jane", hasJID: false}
	result := buildProfile(context.Background(), data)

	if result.Pushname != "Jane" {
		t.Errorf("pushname: %q", result.Pushname)
	}
	if result.JID != "" {
		t.Errorf("jid should be empty, got %q", result.JID)
	}
	if result.AvatarURL != "" {
		t.Errorf("avatar_url should be empty, got %q", result.AvatarURL)
	}
}

func TestBuildProfile_AvatarError(t *testing.T) {
	data := &mockProfileData{
		pushName:  "Test",
		jid:       "1@s.whatsapp.net",
		hasJID:    true,
		avatarErr: errors.New("network error"),
	}
	result := buildProfile(context.Background(), data)

	if result.AvatarURL != "" {
		t.Errorf("avatar_url should be empty on error, got %q", result.AvatarURL)
	}
	if result.Pushname != "Test" {
		t.Errorf("pushname should survive avatar error, got %q", result.Pushname)
	}
}

func TestBuildProfile_ContactError(t *testing.T) {
	data := &mockProfileData{
		pushName:   "Test",
		jid:        "1@s.whatsapp.net",
		hasJID:     true,
		contactErr: errors.New("no contacts"),
	}
	result := buildProfile(context.Background(), data)

	if result.FullName != "" || result.BusinessName != "" {
		t.Errorf("expected empty names on contact error")
	}
}

func TestBuildProfile_EmptyData(t *testing.T) {
	data := &mockProfileData{}
	result := buildProfile(context.Background(), data)

	b, _ := json.Marshal(result)
	expected := `{"pushname":"","avatar_url":"","avatar_id":"","jid":"","full_name":"","business_name":""}`
	if string(b) != expected {
		t.Errorf("expected all-empty JSON, got %s", string(b))
	}
}

func TestBuildProfile_Partial_NilJID(t *testing.T) {
	data := &mockProfileData{
		pushName: "Partial",
		jid:      "",
		hasJID:   false,
	}
	result := buildProfile(context.Background(), data)

	if result.Pushname != "Partial" {
		t.Errorf("pushname: %q", result.Pushname)
	}
	if result.JID != "" {
		t.Errorf("jid should be empty")
	}
}
