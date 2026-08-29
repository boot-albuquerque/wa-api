package appstate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wa-api/internal/headless"
	appport "wa-api/pkg/application/contracts"
	adapter "wa-api/pkg/infra/headless"
	"wa-api/pkg/infra/headless/registry"
)

func cfgFor(string) (headless.StartConfig, error) {
	return headless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

type primerDuplo struct {
	chamadas int
	err      error
}

func (p *primerDuplo) Prime(context.Context, string) (headless.RosterPrimeResult, error) {
	p.chamadas++
	return headless.RosterPrimeResult{}, p.err
}

func comPrimer(p primer) *Syncer {
	s := NewSyncer(adapter.NewSessions(registry.New(1), cfgFor))
	s.newPrimer = func(context.Context, string) (primer, error) { return p, nil }
	return s
}

func TestSatisfazOPortDeSincronizacao(t *testing.T) {
	var s any = NewSyncer(adapter.NewSessions(registry.New(1), cfgFor))
	if _, ok := s.(appport.AppStateSyncer); !ok {
		t.Fatal("não satisfaz AppStateSyncer")
	}
}

// TestOsTresModosCorremAMesmaOperacao documenta o que o adaptador FAZ, em vez de
// deixar a leitura do port sugerir uma distinção que não existe aqui.
//
// Os modos descrevem semântica de patch de app-state, que é conceito de
// protocolo. A página tem um comportamento só: pedir refresh e relatar o que
// mudou. Não há re-snapshot a requisitar nem versão a preservar, porque não há
// fluxo de patch em que se possa estar atrasado.
func TestOsTresModosCorremAMesmaOperacao(t *testing.T) {
	for _, modo := range []string{"if_unsynced", "incremental", "full"} {
		d := &primerDuplo{}
		if err := comPrimer(d).SyncContactRoster(context.Background(), "s1", modo); err != nil {
			t.Fatalf("modo %q: %v", modo, err)
		}
		if d.chamadas != 1 {
			t.Fatalf("modo %q chamou Prime %d vezes", modo, d.chamadas)
		}
	}
}

// TestModoDesconhecidoERecusado: um typo virar no-op silencioso faria o chamador
// acreditar que pediu algo que não pediu.
func TestModoDesconhecidoERecusado(t *testing.T) {
	d := &primerDuplo{}
	err := comPrimer(d).SyncContactRoster(context.Background(), "s1", "completo")
	if err == nil {
		t.Fatal("um modo desconhecido foi aceito")
	}
	if !strings.Contains(err.Error(), "unknown sync mode") {
		t.Fatalf("o erro não nomeia a causa: %v", err)
	}
	if d.chamadas != 0 {
		t.Fatalf("o modo desconhecido chegou a chamar Prime %d vezes", d.chamadas)
	}
}

// A falha da capability propaga. O contrato dela reserva o erro para a página
// recusar ou para o roster ENCOLHER — que é o desfecho que um refresh nunca
// pode produzir em silêncio.
func TestFalhaDaCapabilityPropaga(t *testing.T) {
	d := &primerDuplo{err: errors.New("o roster encolheu")}
	if err := comPrimer(d).SyncContactRoster(context.Background(), "s1", "full"); err == nil {
		t.Fatal("o roster encolher virou sincronização bem-sucedida")
	}
}

// A validação do modo acontece ANTES de tocar na sessão.
func TestValidacaoAconteceAntesDeGastarSlot(t *testing.T) {
	reg := registry.New(1)
	s := NewSyncer(adapter.NewSessions(reg, cfgFor))

	_ = s.SyncContactRoster(context.Background(), "s1", "modo-invalido")

	if reg.Len() != 0 {
		t.Fatalf("Len=%d: um modo inválido consumiu slot de sessão", reg.Len())
	}
}
