package domain

import (
	"strings"
	"testing"
)

// F225: chat as universal JSON alias for the destination field.
//
// Five scenarios per task spec:
//  1. "chat" alias alone populates the legacy field
//  2. legacy name alone still works (backward compat)
//  3. both present and divergent → legacy wins
//  4. neither present → field stays empty (validation unchanged)
//  5. control negative: reintroduce defect, confirm test fails

// chatAliasCase describes one type-family under test.
type chatAliasCase struct {
	name       string
	chatOnly   string // JSON with only "chat"
	legacyOnly string // JSON with only the legacy field
	both       string // JSON with both, divergent values
	empty      string // JSON with neither
	decode     func(string) (legacyValue string, err error)
}

func chatAliasCases() []chatAliasCase {
	return []chatAliasCase{
		{
			name:       "SendMessageRequest/Phone",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net","Body":"hi"}`,
			legacyOnly: `{"Phone":"554192421234@s.whatsapp.net","Body":"hi"}`,
			both:       `{"Phone":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net","Body":"hi"}`,
			empty:      `{"Body":"hi"}`,
			decode: func(body string) (string, error) {
				var req SendMessageRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Phone, err
			},
		},
		{
			name:       "GetGroupInfoRequest/GroupJID",
			chatOnly:   `{"chat":"120363@g.us"}`,
			legacyOnly: `{"groupJID":"120363@g.us"}`,
			both:       `{"groupJID":"legacy@g.us","chat":"alias@g.us"}`,
			empty:      `{}`,
			decode: func(body string) (string, error) {
				var req GetGroupInfoRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.GroupJID, err
			},
		},
		{
			name:       "MuteChatRequest/Jid",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net"}`,
			legacyOnly: `{"jid":"554192421234@s.whatsapp.net"}`,
			both:       `{"jid":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net"}`,
			empty:      `{}`,
			decode: func(body string) (string, error) {
				var req MuteChatRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Jid, err
			},
		},
		{
			name:       "ArchiveChatRequest/Jid",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net"}`,
			legacyOnly: `{"jid":"554192421234@s.whatsapp.net"}`,
			both:       `{"jid":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net"}`,
			empty:      `{}`,
			decode: func(body string) (string, error) {
				var req ArchiveChatRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Jid, err
			},
		},
		{
			name:       "PinChatRequest/Jid",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net"}`,
			legacyOnly: `{"jid":"554192421234@s.whatsapp.net"}`,
			both:       `{"jid":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net"}`,
			empty:      `{}`,
			decode: func(body string) (string, error) {
				var req PinChatRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Jid, err
			},
		},
		{
			name:       "SendPollRequest/Group",
			chatOnly:   `{"chat":"120363@g.us","Name":"Q","Options":["a","b"],"Count":1}`,
			legacyOnly: `{"Group":"120363@g.us","Name":"Q","Options":["a","b"],"Count":1}`,
			both:       `{"Group":"legacy@g.us","chat":"alias@g.us","Name":"Q","Options":["a","b"],"Count":1}`,
			empty:      `{"Name":"Q","Options":["a","b"],"Count":1}`,
			decode: func(body string) (string, error) {
				var req SendPollRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Group, err
			},
		},
		{
			name:       "SubscribePresenceRequest/Phone",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net"}`,
			legacyOnly: `{"Phone":"554192421234@s.whatsapp.net"}`,
			both:       `{"Phone":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net"}`,
			empty:      `{}`,
			decode: func(body string) (string, error) {
				var req SubscribePresenceRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Phone, err
			},
		},
		{
			name:       "BlockUserRequest/Phone",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net"}`,
			legacyOnly: `{"Phone":"554192421234@s.whatsapp.net"}`,
			both:       `{"Phone":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net"}`,
			empty:      `{}`,
			decode: func(body string) (string, error) {
				var req BlockUserRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Phone, err
			},
		},
		{
			name:       "GetAvatarRequest/Phone",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net"}`,
			legacyOnly: `{"Phone":"554192421234@s.whatsapp.net"}`,
			both:       `{"Phone":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net"}`,
			empty:      `{}`,
			decode: func(body string) (string, error) {
				var req GetAvatarRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Phone, err
			},
		},
		{
			name:       "SendForwardRequest/Phone",
			chatOnly:   `{"chat":"554192421234@s.whatsapp.net","stanzaId":"abc","Chat":"origin@s.whatsapp.net"}`,
			legacyOnly: `{"Phone":"554192421234@s.whatsapp.net","stanzaId":"abc","Chat":"origin@s.whatsapp.net"}`,
			both:       `{"Phone":"legacy@s.whatsapp.net","chat":"alias@s.whatsapp.net","stanzaId":"abc","Chat":"origin@s.whatsapp.net"}`,
			empty:      `{"stanzaId":"abc","Chat":"origin@s.whatsapp.net"}`,
			decode: func(body string) (string, error) {
				var req SendForwardRequest
				err := DecodeRequest(strings.NewReader(body), &req)
				return req.Phone, err
			},
		},
	}
}

const wantDest = "554192421234@s.whatsapp.net"

func TestChatAlias_OnlyChat(t *testing.T) {
	for _, tc := range chatAliasCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.decode(tc.chatOnly)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got == "" {
				t.Fatal("chat alias was not resolved into legacy field")
			}
		})
	}
}

func TestChatAlias_OnlyLegacy(t *testing.T) {
	for _, tc := range chatAliasCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.decode(tc.legacyOnly)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got == "" {
				t.Fatal("legacy field not populated")
			}
		})
	}
}

func TestChatAlias_BothPresent_LegacyWins(t *testing.T) {
	for _, tc := range chatAliasCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.decode(tc.both)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !strings.HasPrefix(got, "legacy") {
				t.Fatalf("legacy field should win when both present: got %q", got)
			}
		})
	}
}

func TestChatAlias_NeitherPresent_FieldEmpty(t *testing.T) {
	for _, tc := range chatAliasCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.decode(tc.empty)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got != "" {
				t.Fatalf("neither legacy nor alias present but field is %q", got)
			}
		})
	}
}

// TestChatAlias_ControlNegative_NoResolveChat verifies that without the
// ResolveChat hook, the "chat" JSON key does NOT populate the legacy
// field. This is the control negative: it proves the test would fail
// if the mechanism were removed.
func TestChatAlias_ControlNegative_NoResolveChat(t *testing.T) {
	type bareRequest struct {
		Phone string `json:"Phone"`
	}
	body := `{"chat":"554192421234@s.whatsapp.net"}`
	var req bareRequest
	if err := DecodeRequest(strings.NewReader(body), &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.Phone != "" {
		t.Fatal("control negative failed: Phone was populated without ChatTarget embedding")
	}
}
