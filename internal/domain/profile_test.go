package domain_test

import (
	"encoding/json"
	"testing"

	"wa-api/internal/domain"
)

func TestProfileStruct_AllFields(t *testing.T) {
	p := domain.Profile{
		Pushname:     "Test Push",
		AvatarURL:    "https://example.com/avatar.jpg",
		AvatarID:     "av-123",
		JID:          "5511987654321@s.whatsapp.net",
		FullName:     "Test User",
		BusinessName: "Test Business",
	}

	if p.Pushname != "Test Push" {
		t.Errorf("Pushname: got %q", p.Pushname)
	}
	if p.AvatarURL != "https://example.com/avatar.jpg" {
		t.Errorf("AvatarURL: got %q", p.AvatarURL)
	}
	if p.AvatarID != "av-123" {
		t.Errorf("AvatarID: got %q", p.AvatarID)
	}
	if p.JID != "5511987654321@s.whatsapp.net" {
		t.Errorf("JID: got %q", p.JID)
	}
	if p.FullName != "Test User" {
		t.Errorf("FullName: got %q", p.FullName)
	}
	if p.BusinessName != "Test Business" {
		t.Errorf("BusinessName: got %q", p.BusinessName)
	}
}

func TestProfileStruct_Empty(t *testing.T) {
	p := domain.Profile{}

	if p.Pushname != "" {
		t.Error("expected empty Pushname")
	}
	if p.AvatarURL != "" {
		t.Error("expected empty AvatarURL")
	}
	if p.AvatarID != "" {
		t.Error("expected empty AvatarID")
	}
	if p.JID != "" {
		t.Error("expected empty JID")
	}
	if p.FullName != "" {
		t.Error("expected empty FullName")
	}
	if p.BusinessName != "" {
		t.Error("expected empty BusinessName")
	}
}

func TestProfileJSON_Serialization(t *testing.T) {
	p := domain.Profile{
		Pushname:     "Jane",
		AvatarURL:    "https://img.example.com/123.jpg",
		AvatarID:     "abc456",
		JID:          "5511999999999@s.whatsapp.net",
		FullName:     "Jane Doe",
		BusinessName: "Jane's Shop",
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed["pushname"] != "Jane" {
		t.Errorf("pushname: %q", parsed["pushname"])
	}
	if parsed["avatar_url"] != "https://img.example.com/123.jpg" {
		t.Errorf("avatar_url: %q", parsed["avatar_url"])
	}
	if parsed["avatar_id"] != "abc456" {
		t.Errorf("avatar_id: %q", parsed["avatar_id"])
	}
	if parsed["jid"] != "5511999999999@s.whatsapp.net" {
		t.Errorf("jid: %q", parsed["jid"])
	}
	if parsed["full_name"] != "Jane Doe" {
		t.Errorf("full_name: %q", parsed["full_name"])
	}
	if parsed["business_name"] != "Jane's Shop" {
		t.Errorf("business_name: %q", parsed["business_name"])
	}
}

func TestProfileJSON_EmptyFields(t *testing.T) {
	p := domain.Profile{
		Pushname: "OnlyPush",
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]string
	_ = json.Unmarshal(b, &parsed)

	if parsed["pushname"] != "OnlyPush" {
		t.Errorf("pushname: %q", parsed["pushname"])
	}
	if parsed["avatar_url"] != "" {
		t.Errorf("expected empty avatar_url, got %q", parsed["avatar_url"])
	}
}
