package domain_test

import (
	"errors"
	"strings"
	"testing"

	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
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
		// Nome vazio é AUSENTE, não inválido: os dois erros têm correções
		// diferentes. O código é afirmado em TestValidatePrivacySetting_
		// AusenteVsInvalido.
		{"empty setting", "", "all", true, "missing privacy setting name"},
		// readreceipts aceita apenas all/none: "contacts" é válido para
		// outras configurações, e é exatamente esse cruzamento que a
		// validação por tabela existe para pegar.
		{"value valid elsewhere", "readreceipts", "contacts", true, "invalid value"},
		{"online rejects contacts", "online", "contacts", true, "invalid value"},
		{"calladd rejects none", "calladd", "none", true, "invalid value"},
		{"empty value", "last", "", true, "missing privacy setting value"},
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

// TestValidatePrivacySetting_AusenteVsInvalido trava a distinção que o
// consumidor automatizado usa: ramificar sobre error.code tem de separar
// "não mandaste o campo" de "mandaste um valor errado", porque as correções
// são diferentes. Antes desta correção os quatro casos abaixo devolviam
// invalid_privacy_setting / invalid_privacy_value indistintamente.
func TestValidatePrivacySetting_AusenteVsInvalido(t *testing.T) {
	tests := []struct {
		name     string
		setting  string
		value    string
		wantCode string
		// wantMsgPart, quando não vazio, é o eco do valor recebido: é o que
		// permite ao consumidor ver o próprio erro de digitação.
		wantMsgPart string
	}{
		{"setting ausente", "", "all", domain.MissingPrivacySettingCode, ""},
		{"value ausente", "last", "", domain.MissingPrivacyValueCode, ""},
		{"setting invalido", "xpto", "all", domain.InvalidPrivacySettingCode, `"xpto"`},
		{"setting maiusculas", "LAST", "all", domain.InvalidPrivacySettingCode, `"LAST"`},
		{"value invalido", "last", "xpto", domain.InvalidPrivacyValueCode, `"xpto"`},
		{"value maiusculas", "last", "ALL", domain.InvalidPrivacyValueCode, `"ALL"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidatePrivacySetting(tt.setting, tt.value)
			if err == nil {
				t.Fatalf("ValidatePrivacySetting(%q, %q) = nil, quero recusa", tt.setting, tt.value)
			}
			var appErr *apperr.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("erro nao e' um *apperr.AppError: %v", err)
			}
			if appErr.Code != tt.wantCode {
				t.Fatalf("Code = %q, quero %q", appErr.Code, tt.wantCode)
			}
			if appErr.Category != apperr.CategoryValidation {
				t.Errorf("Category = %q, quero %q", appErr.Category, apperr.CategoryValidation)
			}
			if tt.wantMsgPart != "" && !strings.Contains(err.Error(), tt.wantMsgPart) {
				t.Errorf("mensagem %q nao ecoa o valor recebido %s", err.Error(), tt.wantMsgPart)
			}
		})
	}
}
