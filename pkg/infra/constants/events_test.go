package constants

import (
	"sort"
	"testing"

	"wa-api/pkg/domain"
)

// TestIsValidEventType exercita a tabela que init() constrói. O pacote não
// tinha nenhum teste — a validação de tipo de evento decide o que o webhook
// entrega, e uma entrada perdida em SupportedEventTypes passava silenciosa.
func TestIsValidEventType(t *testing.T) {
	tests := []struct {
		name  string
		event string
		want  bool
	}{
		{"message", "Message", true},
		{"all", "All", true},
		{"newsletter", "NewsletterJoin", true},
		{"call", "CallOffer", true},
		{"unknown", "NotAnEvent", false},
		{"empty", "", false},
		{"wrong case", "message", false},
		{"trailing space", "Message ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidEventType(tt.event); got != tt.want {
				t.Errorf("IsValidEventType(%q) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

// TestEverySupportedEventTypeIsValid checks that init() built the map
// correctly: every declared element validates, and no duplicates exist.
func TestEverySupportedEventTypeIsValid(t *testing.T) {
	if len(SupportedEventTypes) == 0 {
		t.Fatal("SupportedEventTypes is empty")
	}
	seen := make(map[string]bool, len(SupportedEventTypes))
	for _, event := range SupportedEventTypes {
		if !IsValidEventType(event) {
			t.Errorf("declared event %q is not accepted by IsValidEventType", event)
		}
		if seen[event] {
			t.Errorf("event %q appears more than once in SupportedEventTypes", event)
		}
		seen[event] = true
	}
}

// TestConstantsAndDomainDescribeSameSet locks the invariant: the infra
// copy must contain exactly the same elements as domain's source list.
// A divergence (element in one but not the other) fails with the element
// named, so the fix is obvious.
func TestConstantsAndDomainDescribeSameSet(t *testing.T) {
	domainSet := make(map[string]struct{}, len(domain.SupportedEventTypes))
	for _, e := range domain.SupportedEventTypes {
		domainSet[e] = struct{}{}
	}
	constantsSet := make(map[string]struct{}, len(SupportedEventTypes))
	for _, e := range SupportedEventTypes {
		constantsSet[e] = struct{}{}
	}

	for e := range constantsSet {
		if _, ok := domainSet[e]; !ok {
			t.Errorf("constants has %q but domain does not", e)
		}
	}
	for e := range domainSet {
		if _, ok := constantsSet[e]; !ok {
			t.Errorf("domain has %q but constants does not", e)
		}
	}

	if len(SupportedEventTypes) != len(domain.SupportedEventTypes) {
		t.Errorf("length mismatch: constants=%d, domain=%d",
			len(SupportedEventTypes), len(domain.SupportedEventTypes))
	}
}

// TestIsValidEventTypeAgreesWithDomain verifies that both packages agree
// on membership for every declared event AND for at least one outsider.
func TestIsValidEventTypeAgreesWithDomain(t *testing.T) {
	outsiders := []string{"NotAnEvent", "", "message", "All "}
	all := make([]string, 0, len(domain.SupportedEventTypes)+len(outsiders))
	all = append(all, domain.SupportedEventTypes...)
	all = append(all, outsiders...)

	sort.Strings(all)

	for _, e := range all {
		got := IsValidEventType(e)
		want := domain.IsValidEventType(e)
		if got != want {
			t.Errorf("IsValidEventType(%q): constants=%v, domain=%v", e, got, want)
		}
	}
}
