package bootstrap

import (
	"testing"

	"wa-api/pkg/domain"
)

// TestEngineDomainForCoversEveryConfiguredEngine trava a ÚNICA travessia entre
// os dois vocabulários de engine que existem hoje.
//
// A dualidade é temporária e está registrada no HOUSEKEEP: este pacote fala
// "wanoise"/"headless" (o que já está nos ambientes em produção) e o domínio
// fala "wa_noise"/"wa_headless" (o contrato público persistido). Se alguém
// acrescentar um engine de configuração e esquecer o mapeamento, o backfill
// gravaria o engine errado em TODA linha — e este teste é o que impede isso de
// passar em silêncio.
func TestEngineDomainForCoversEveryConfiguredEngine(t *testing.T) {
	// A lista vem das constantes deste pacote, não de literais: acrescentar um
	// engine sem acrescentar aqui deixaria o teste a medir o conjunto antigo.
	cases := map[string]domain.Engine{
		EngineWaNoise:    domain.EngineWaNoise,
		EngineWaHeadless: domain.EngineWaHeadless,
	}
	for infra, want := range cases {
		got, err := engineDomainFor(infra)
		if err != nil {
			t.Fatalf("engineDomainFor(%q): %v", infra, err)
		}
		if got != want {
			t.Errorf("engineDomainFor(%q) = %q, want %q", infra, got, want)
		}
	}
}

// TestEngineDomainForRejectsUnknown: sem fallback silencioso, pela mesma razão
// da decisão 94.
func TestEngineDomainForRejectsUnknown(t *testing.T) {
	for _, bad := range []string{"", "wa_noise", "wa_headless", "headles", "foobar"} {
		got, err := engineDomainFor(bad)
		if err == nil {
			t.Errorf("engineDomainFor(%q) = %q, want error", bad, got)
		}
		if got != "" {
			t.Errorf("engineDomainFor(%q) = %q, want empty on error", bad, got)
		}
	}
}

// TestEngineSelectionFeedsTheBackfill liga as duas pontas: o que sai do parsing
// de WA_API_ENGINE_HEADLESS_SESSIONS é exatamente o que o backfill consome.
//
// Sem isto, o parsing e o backfill poderiam divergir (por exemplo, o backfill
// a receber a CSV crua em vez da lista já validada) e cada um passaria no seu
// próprio teste.
func TestEngineSelectionFeedsTheBackfill(t *testing.T) {
	t.Setenv(envEngineDefault, "")
	t.Setenv(envEngineSessions, "sess-b, sess-a ,sess-c")

	sel, err := engineSelectionConfigurada()
	if err != nil {
		t.Fatalf("engineSelectionConfigurada: %v", err)
	}

	defaultEngine, err := engineDomainFor(sel.Default())
	if err != nil {
		t.Fatalf("engineDomainFor(%q): %v", sel.Default(), err)
	}
	if defaultEngine != domain.EngineWaNoise {
		t.Errorf("default engine = %q, want %q", defaultEngine, domain.EngineWaNoise)
	}

	// Ordenada e com os espaços já removidos — é essa a lista que o backfill
	// usa para casar `users.id`, e um id com espaço não casaria com nada.
	got := sel.SessoesEmHeadless()
	want := []string{"sess-a", "sess-b", "sess-c"}
	if len(got) != len(want) {
		t.Fatalf("SessoesEmHeadless() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SessoesEmHeadless() = %v, want %v", got, want)
		}
	}
}
