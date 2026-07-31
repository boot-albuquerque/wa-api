package domain_test

import (
	"testing"

	"wa-api/internal/domain"
)

func TestIsValidEventType(t *testing.T) {
	tests := []struct {
		name  string
		event string
		want  bool
	}{
		{"Message", "Message", true},
		{"All", "All", true},
		{"Presence", "Presence", true},
		{"PrivacySettings", "PrivacySettings", true},
		{"invalid", "InvalidEvent", false},
		{"empty", "", false},
		{"caseSensitive", "message", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.IsValidEventType(tt.event)
			if got != tt.want {
				t.Errorf("IsValidEventType(%q) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}
