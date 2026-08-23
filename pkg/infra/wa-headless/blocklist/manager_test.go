package blocklist

import (
	"context"
	"strings"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// Este port é satisfeito por INTEIRO — os três verbos estão PROVEN no LEDGER
// (block e unblock na H59, a leitura na H146). Não há assimetria de capacidade
// aqui, e o teste diz isso em vez de o deixar implícito.
func TestSatisfazOPortInteiro(t *testing.T) {
	var m any = NewManager(registry.New(1), cfgFor)
	if _, ok := m.(appport.BlocklistManager); !ok {
		t.Fatal("não satisfaz BlocklistManager")
	}
}

// TestGrupoERecusadoAntesDeGastarSlot: um grupo não pode ser bloqueado, e a
// recusa acontece na conversão de identidade — antes de qualquer sessão.
func TestGrupoERecusadoAntesDeGastarSlot(t *testing.T) {
	reg := registry.New(1)
	m := NewManager(reg, cfgFor)

	_, err := m.UpdateBlocklist(context.Background(), "s1", domain.JID("status@broadcast"), true)
	if err == nil {
		t.Fatal("um broadcast foi aceito como alvo de bloqueio")
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a recusa consumiu slot de sessão", reg.Len())
	}
}

// TestDHashEVazioEIssoEDeliberado trava a assimetria de DADO, que é diferente
// da assimetria de capacidade.
//
// O socket versiona a blocklist com um hash que o servidor manda; a página não
// expõe equivalente — entrega a coleção, não a versão dela. Um valor inventado
// seria bem-formado e falso, e um chamador que comparasse dois deles concluiria
// "não mudou" a partir de duas listas diferentes.
//
// O teste existe porque o vazio é uma DECISÃO, e sem ele alguém o lê como
// esquecimento e "corrige".
func TestDHashEVazioEIssoEDeliberado(t *testing.T) {
	if dhashUnavailable != "" {
		t.Fatalf("dhashUnavailable = %q: se a página passou a expor uma versão da "+
			"blocklist, isto é uma mudança de capacidade e precisa de medição, "+
			"não de um valor novo aqui", dhashUnavailable)
	}
}

// A conversão de identidade acontece ANTES de resolver a sessão, pela mesma
// razão dos outros adaptadores: um pedido que nunca poderia funcionar não gasta
// capacidade limitada.
func TestIdentidadeInvalidaNaoChegaAoRegistry(t *testing.T) {
	reg := registry.New(1)
	m := NewManager(reg, cfgFor)

	_, err := m.UpdateBlocklist(context.Background(), "s1", domain.JID(""), true)
	if err == nil {
		t.Fatal("JID vazio foi aceito")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("erro não nomeia a causa: %v", err)
	}
	if reg.Len() != 0 {
		t.Fatalf("Len=%d", reg.Len())
	}
}
