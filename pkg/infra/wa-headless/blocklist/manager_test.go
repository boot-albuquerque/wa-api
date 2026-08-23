package blocklist

import (
	"context"
	"errors"
	"strings"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	adapter "wa-api/pkg/infra/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgFor(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// Este port é satisfeito por INTEIRO — os três verbos estão PROVEN no LEDGER
// (block e unblock na H59, a leitura na H146). Não há assimetria de capacidade
// aqui, e o teste diz isso em vez de o deixar implícito.
func TestSatisfazOPortInteiro(t *testing.T) {
	var m any = NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := m.(appport.BlocklistManager); !ok {
		t.Fatal("não satisfaz BlocklistManager")
	}
}

// TestGrupoERecusadoAntesDeGastarSlot: um grupo não pode ser bloqueado, e a
// recusa acontece na conversão de identidade — antes de qualquer sessão.
func TestGrupoERecusadoAntesDeGastarSlot(t *testing.T) {
	reg := registry.New(1)
	m := NewManager(adapter.NewSessions(reg, cfgFor))

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
	m := NewManager(adapter.NewSessions(reg, cfgFor))

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

// blockerDuplo registra QUAIS verbos foram chamados, porque a ordem e a escolha
// são as regras deste adaptador — a capability já tem os seus próprios testes.
type blockerDuplo struct {
	chamadas []string
	lista    []string
	err      error
}

func (b *blockerDuplo) Block(context.Context, string, string) (waheadless.BlockResult, error) {
	b.chamadas = append(b.chamadas, "block")
	return waheadless.BlockResult{}, b.err
}

func (b *blockerDuplo) Unblock(context.Context, string, string) (waheadless.BlockResult, error) {
	b.chamadas = append(b.chamadas, "unblock")
	return waheadless.BlockResult{}, b.err
}

func (b *blockerDuplo) List(context.Context, string) ([]string, error) {
	b.chamadas = append(b.chamadas, "list")
	return b.lista, b.err
}

func comBlocker(b blocker) *Manager {
	m := NewManager(adapter.NewSessions(registry.New(1), cfgFor))
	m.newBlocker = func(context.Context, string) (blocker, error) { return b, nil }
	return m
}

// TestBloquearLeDeVoltaAListaResultante trava a pós-condição da invariante 14 na
// fronteira: o port promete a lista resultante, e o Result da capability carrega
// SÓ TAMANHOS, nunca as entradas. Sem a releitura, o adaptador devolveria uma
// lista vazia com ar de resposta.
func TestBloquearLeDeVoltaAListaResultante(t *testing.T) {
	d := &blockerDuplo{lista: []string{"5511999999999@c.us"}}
	m := comBlocker(d)

	got, err := m.UpdateBlocklist(context.Background(), "s1", domain.JID("5511999999999@c.us"), true)
	if err != nil {
		t.Fatalf("UpdateBlocklist: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Entries=%v: a lista resultante não foi lida de volta", got.Entries)
	}
	if len(d.chamadas) != 2 || d.chamadas[0] != "block" || d.chamadas[1] != "list" {
		t.Fatalf("chamadas=%v, quero block seguido de list", d.chamadas)
	}
}

// Desbloquear chama o verbo INVERSO. Trocar os dois é o bug que nenhum teste de
// tipo apanha, porque as assinaturas são idênticas.
func TestDesbloquearChamaOVerboInverso(t *testing.T) {
	d := &blockerDuplo{}
	m := comBlocker(d)

	if _, err := m.UpdateBlocklist(context.Background(), "s1", domain.JID("5511999999999@c.us"), false); err != nil {
		t.Fatalf("UpdateBlocklist: %v", err)
	}
	if len(d.chamadas) == 0 || d.chamadas[0] != "unblock" {
		t.Fatalf("chamadas=%v, quero unblock primeiro", d.chamadas)
	}
}

// A leitura simples devolve o que a página deu, com o DHash honesto.
func TestGetBlocklistDevolveALista(t *testing.T) {
	m := comBlocker(&blockerDuplo{lista: []string{"a@c.us", "b@lid"}})

	got, err := m.GetBlocklist(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetBlocklist: %v", err)
	}
	if len(got.JIDs) != 2 {
		t.Fatalf("JIDs=%v", got.JIDs)
	}
	if got.DHash != "" {
		t.Fatalf("DHash=%q: esta página não versiona a blocklist", got.DHash)
	}
}

// A falha ao resolver a CONFIGURAÇÃO propaga, em vez de virar lista vazia.
func TestFalhaDeConfiguracaoPropaga(t *testing.T) {
	m := NewManager(adapter.NewSessions(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil para esta sessão")
	}))
	if _, err := m.GetBlocklist(context.Background(), "s1"); err == nil {
		t.Fatal("falha de configuração virou blocklist vazia")
	}
}
