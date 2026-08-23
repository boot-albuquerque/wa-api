package waheadless

import (
	"context"
	"errors"
	"strings"
	"testing"

	waheadless "wa-api/internal/wa-headless"
	"wa-api/pkg/infra/wa-headless/registry"
)

func cfgOK(string) (waheadless.StartConfig, error) {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}, nil
}

// A falha ao resolver a CONFIGURAÇÃO propaga com a causa nomeada. Antes desta
// seam, esta regra existia em seis cópias e nenhuma era testada nas seis.
func TestFalhaDeConfiguracaoPropagaComACausa(t *testing.T) {
	s := NewSessions(registry.New(1), func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil para esta sessão")
	})

	_, err := s.Evaluator(context.Background(), "s1")
	if err == nil {
		t.Fatal("falha de configuração virou sessão utilizável")
	}
	if !strings.Contains(err.Error(), "config for session") {
		t.Fatalf("o erro não nomeia a causa: %v", err)
	}
}

// TestConfiguracaoERESOLVIDAANTESDeGastarSlot trava a ORDEM, que é o que a
// invariante do teto exige: uma sessão que ninguém configurou não pode consumir
// a capacidade de que uma sessão funcional precisa.
func TestConfiguracaoERESOLVIDAANTESDeGastarSlot(t *testing.T) {
	reg := registry.New(1)
	s := NewSessions(reg, func(string) (waheadless.StartConfig, error) {
		return waheadless.StartConfig{}, errors.New("sem perfil")
	})

	_, _ = s.Evaluator(context.Background(), "s1")

	if reg.Len() != 0 {
		t.Fatalf("Len=%d: a sessão sem configuração consumiu um slot que uma "+
			"sessão funcional precisaria", reg.Len())
	}
}

// A recusa por CAPACIDADE chega ao chamador como erro, e não como silêncio.
// "Esta máquina está cheia" é informação acionável; uma sessão ausente sem
// motivo não é.
func TestRecusaPorCapacidadeChegaAoChamador(t *testing.T) {
	reg := registry.New(1)
	if _, err := reg.Acquire("ocupante", waheadless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	s := NewSessions(reg, cfgOK)
	_, err := s.Evaluator(context.Background(), "excedente")
	if !errors.Is(err, registry.ErrAtCapacity) {
		t.Fatalf("got %v, want ErrAtCapacity", err)
	}
}

// EnsureSession responde POSSE sem bootar. A ADR-0005 D6 separa posse de
// prontidão, e uma guarda que subisse browser para responder transformaria uma
// checagem barata num minuto de trabalho.
func TestEnsureSessionRespondePosseSemBootar(t *testing.T) {
	reg := registry.New(2)
	s := NewSessions(reg, cfgOK)

	if err := s.EnsureSession(context.Background(), "desconhecida"); !errors.Is(err, registry.ErrUnknownSession) {
		t.Fatalf("got %v, want ErrUnknownSession", err)
	}
	if _, err := reg.Acquire("conhecida", waheadless.StartConfig{}, registry.KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := s.EnsureSession(context.Background(), "conhecida"); err != nil {
		t.Fatalf("posse negada para sessão detida: %v", err)
	}
}
