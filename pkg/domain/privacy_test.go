package domain_test

import (
	"strings"
	"testing"

	"wa-api/pkg/domain"
)

// TestValidatePrivacySetting cobre a tabela que o use case de privacidade
// consulta. Os dois ramos de rejeição — nome desconhecido e valor fora do
// conjunto daquele nome — produzem mensagens diferentes de propósito: o
// chamador precisa saber qual dos dois errou.
func TestValidatePrivacySetting(t *testing.T) {
	tests := []struct {
		name        string
		setting     string
		value       string
		wantErr     bool
		wantErrPart string
	}{
		{"groupadd all", "groupadd", "all", false, ""},
		{"groupadd contact_blacklist", "groupadd", "contact_blacklist", false, ""},
		{"last none", "last", "none", false, ""},
		{"status contacts", "status", "contacts", false, ""},
		{"profile all", "profile", "all", false, ""},
		{"readreceipts all", "readreceipts", "all", false, ""},
		{"readreceipts none", "readreceipts", "none", false, ""},
		{"online match_last_seen", "online", "match_last_seen", false, ""},
		{"calladd known", "calladd", "known", false, ""},

		{"unknown setting", "nosuchsetting", "all", true, "invalid privacy setting name"},
		{"empty setting", "", "all", true, "invalid privacy setting name"},
		// readreceipts aceita apenas all/none: "contacts" é válido para
		// outras configurações, e é exatamente esse cruzamento que a
		// validação por tabela existe para pegar.
		{"value valid elsewhere", "readreceipts", "contacts", true, "invalid value"},
		{"online rejects contacts", "online", "contacts", true, "invalid value"},
		{"calladd rejects none", "calladd", "none", true, "invalid value"},
		{"empty value", "last", "", true, "invalid value"},
		{"wrong case value", "last", "ALL", true, "invalid value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidatePrivacySetting(tt.setting, tt.value)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidatePrivacySetting(%q, %q) error = %v, wantErr = %v",
					tt.setting, tt.value, err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantErrPart)
			}
		})
	}
}
