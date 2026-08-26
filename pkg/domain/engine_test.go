package domain

import (
	"errors"
	"testing"
)

// TestEngineIsValidForCreate trava o contrato de CRIAÇÃO: só os dois
// transportes reais entram, e `legacy_unknown` fica de fora mesmo sendo um
// valor que o domínio conhece.
func TestEngineIsValidForCreate(t *testing.T) {
	cases := []struct {
		engine Engine
		want   bool
	}{
		{EngineWaNoise, true},
		{EngineWaHeadless, true},
		{EngineLegacyUnknown, false},
		{Engine("foobar"), false},
		{Engine(""), false},
		// As strings de configuração de infraestrutura NÃO são valores de
		// domínio. Se algum dia alguém as passar direto, tem de falhar.
		{Engine("wanoise"), false},
		{Engine("headless"), false},
		// Sem reparo de entrada: maiúscula e espaço são recusa, não conserto.
		{Engine("WA_NOISE"), false},
		{Engine(" wa_noise"), false},
	}
	for _, c := range cases {
		if got := c.engine.IsValidForCreate(); got != c.want {
			t.Errorf("Engine(%q).IsValidForCreate() = %v, want %v", string(c.engine), got, c.want)
		}
	}
}

// TestEngineIsKnown separa a pergunta da LEITURA da pergunta da criação:
// legacy_unknown é um estado legítimo de linha antiga.
func TestEngineIsKnown(t *testing.T) {
	cases := []struct {
		engine Engine
		want   bool
	}{
		{EngineWaNoise, true},
		{EngineWaHeadless, true},
		{EngineLegacyUnknown, true},
		{Engine("foobar"), false},
		{Engine(""), false},
	}
	for _, c := range cases {
		if got := c.engine.IsKnown(); got != c.want {
			t.Errorf("Engine(%q).IsKnown() = %v, want %v", string(c.engine), got, c.want)
		}
	}
}

// TestEngineValuesAreSnakeCase trava o TEXTO do contrato público. Sem isto,
// renomear uma constante mudaria o que está gravado no banco sem que nenhum
// teste morresse.
func TestEngineValuesAreSnakeCase(t *testing.T) {
	if EngineWaNoise.String() != "wa_noise" {
		t.Errorf("EngineWaNoise = %q, want %q", EngineWaNoise, "wa_noise")
	}
	if EngineWaHeadless.String() != "wa_headless" {
		t.Errorf("EngineWaHeadless = %q, want %q", EngineWaHeadless, "wa_headless")
	}
	if EngineLegacyUnknown.String() != "legacy_unknown" {
		t.Errorf("EngineLegacyUnknown = %q, want %q", EngineLegacyUnknown, "legacy_unknown")
	}
}

func TestParseEngine(t *testing.T) {
	for _, raw := range []string{"wa_noise", "wa_headless"} {
		got, err := ParseEngine(raw)
		if err != nil {
			t.Fatalf("ParseEngine(%q): %v", raw, err)
		}
		if got.String() != raw {
			t.Errorf("ParseEngine(%q) = %q", raw, got)
		}
	}

	for _, raw := range []string{"legacy_unknown", "", "foobar", "wanoise", "headless"} {
		got, err := ParseEngine(raw)
		if !errors.Is(err, ErrInvalidEngine) {
			t.Errorf("ParseEngine(%q) err = %v, want ErrInvalidEngine", raw, err)
		}
		if got != "" {
			t.Errorf("ParseEngine(%q) = %q, want empty on error", raw, got)
		}
	}
}
