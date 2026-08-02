package constants

import "testing"

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

// TestEverySupportedEventTypeIsValid é o controle de init(): se a construção
// do mapa deixar de rodar, ou rodar sobre uma lista diferente, todo elemento
// declarado deixa de validar. Também pega duplicatas na lista, que seriam
// invisíveis pelo mapa.
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
