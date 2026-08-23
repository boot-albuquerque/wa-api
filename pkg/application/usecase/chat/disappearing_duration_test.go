package chat

import (
	"testing"
	"time"
)

func TestParseDisappearingDuration(t *testing.T) {
	valid := []struct {
		input string
		want  time.Duration
	}{
		{"0", 0},
		{"off", 0},
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"90d", 90 * 24 * time.Hour},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.input, func(t *testing.T) {
			got, ok := parseDisappearingDuration(tc.input)
			if !ok {
				t.Fatalf("parseDisappearingDuration(%q) returned ok=false, want true", tc.input)
			}
			if got != tc.want {
				t.Fatalf("parseDisappearingDuration(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}

	invalid := []string{"", "30d", "1h", "365d", "24H", "7D", "forever", "true"}
	for _, input := range invalid {
		t.Run("invalid/"+input, func(t *testing.T) {
			_, ok := parseDisappearingDuration(input)
			if ok {
				t.Fatalf("parseDisappearingDuration(%q) returned ok=true, want false", input)
			}
		})
	}
}
