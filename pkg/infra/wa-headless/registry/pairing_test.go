package registry

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// relogio é um relógio controlado. O prazo se testa pela REGRA, e um teste que
// dorme para provar uma expiração prova apenas que dormir funciona.
type relogio struct{ t time.Time }

func (c *relogio) agora() time.Time       { return c.t }
func (c *relogio) avanca(d time.Duration) { c.t = c.t.Add(d) }

func novoRelogio() *relogio {
	return &relogio{t: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)}
}

// TestPareamentoNaoConsomeSlotOperacional é a asserção central da decisão 77.
//
// Uma sessão em QR espera por um HUMANO, e a invariante do projeto é que nada
// que espere por relógio ou par morto ocupe slot limitado — uma pessoa é pior
// que um relógio. Se o pareamento contasse no pool operacional, duas pessoas a
// parear com teto 2 parariam todo o tráfego pareado.
func TestPareamentoNaoConsomeSlotOperacional(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(2, 2, time.Minute, c.agora)

	for _, id := range []string{"p1", "p2"} {
		if _, err := r.Acquire(id, cfg(), KindPairing); err != nil {
			t.Fatalf("Acquire pareamento %q: %v", id, err)
		}
	}
	// O pool operacional continua INTEIRO.
	for _, id := range []string{"op1", "op2"} {
		if _, err := r.Acquire(id, cfg(), KindOperational); err != nil {
			t.Fatalf("pareamento comeu slot operacional: %q recusado com %v", id, err)
		}
	}
	if got := r.LenKind(KindOperational); got != 2 {
		t.Fatalf("operacionais=%d, quero 2", got)
	}
	if got := r.LenKind(KindPairing); got != 2 {
		t.Fatalf("pareamentos=%d, quero 2", got)
	}
}

// A quota de pareamento tem teto PRÓPRIO, senão ela não é quota.
func TestQuotaDePareamentoTemTetoProprio(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(4, 1, time.Minute, c.agora)

	if _, err := r.Acquire("p1", cfg(), KindPairing); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	_, err := r.Acquire("p2", cfg(), KindPairing)
	if !errors.Is(err, ErrPairingAtCapacity) {
		t.Fatalf("got %v, want ErrPairingAtCapacity", err)
	}
	// E o erro é DISTINTO do da capacidade operacional: um diz que a máquina
	// está ocupada a servir, o outro que há gente demais a meio de parear.
	if errors.Is(err, ErrAtCapacity) {
		t.Fatal("os dois esgotamentos são o MESMO erro; um operador não " +
			"consegue distinguir qual dos dois limites bateu")
	}
}

// TestRestartDeSessoesPareadasNaoPassaPelaQuotaDePareamento é a regra 2 do
// CLAUDE.md — medir onde o mecanismo PIORA.
//
// A entrada que faria esta proteção virar o problema é o reinício do processo
// com N sessões JÁ pareadas: se toda sessão nova entrasse pela quota de
// pareamento, a restauração bateria numa quota deliberadamente pequena e o
// teto protetor viraria a indisponibilidade. Por isso o tipo é explícito no
// Acquire e nunca inferido.
func TestRestartDeSessoesPareadasNaoPassaPelaQuotaDePareamento(t *testing.T) {
	c := novoRelogio()
	// Quota de pareamento de UM, pool operacional de quatro: o cenário exato.
	r := NewWithQuotas(4, 1, time.Minute, c.agora)

	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("restaurada%d", i)
		if _, err := r.Acquire(id, cfg(), KindOperational); err != nil {
			t.Fatalf("restaurar %q falhou: %v — a quota de pareamento está no "+
				"caminho da restauração, e o teto virou a indisponibilidade", id, err)
		}
	}
	if got := r.LenKind(KindPairing); got != 0 {
		t.Fatalf("pareamentos=%d depois de restaurar só sessões pareadas", got)
	}
}

// Promover é o que um pareamento bem-sucedido significa para a capacidade.
func TestPromoteMoveDeQuota(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(2, 2, time.Minute, c.agora)

	if _, err := r.Acquire("p1", cfg(), KindPairing); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := r.Promote("p1"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if got := r.LenKind(KindPairing); got != 0 {
		t.Fatalf("pareamentos=%d depois de promover, quero 0", got)
	}
	if got := r.LenKind(KindOperational); got != 1 {
		t.Fatalf("operacionais=%d depois de promover, quero 1", got)
	}
}

// Com o pool operacional cheio, promover FALHA e a sessão FICA na quota de
// pareamento. Uma promoção que largasse a entrada em silêncio vazaria um
// browser vivo sem slot nenhum a contá-lo.
func TestPromoteComOperacionalCheioNaoPerdeASessao(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(1, 2, time.Minute, c.agora)

	if _, err := r.Acquire("op1", cfg(), KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := r.Acquire("p1", cfg(), KindPairing); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := r.Promote("p1"); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("got %v, want ErrAtCapacity", err)
	}
	if !r.Holds("p1") {
		t.Fatal("a sessão sumiu do registry ao falhar a promoção: um browser " +
			"vivo sem slot a contá-lo")
	}
	if got := r.LenKind(KindPairing); got != 1 {
		t.Fatalf("pareamentos=%d, quero 1 — a sessão tem de FICAR na quota", got)
	}
}

// Promover algo que já é operacional é idempotente, e não erro: um observador
// que veja o pareamento concluir duas vezes não pode derrubar a sessão.
func TestPromoteEIdempotente(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(2, 2, time.Minute, c.agora)
	if _, err := r.Acquire("p1", cfg(), KindPairing); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := r.Promote("p1"); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if err := r.Promote("p1"); err != nil {
		t.Fatalf("segunda promoção deu erro: %v", err)
	}
	if got := r.LenKind(KindOperational); got != 1 {
		t.Fatalf("operacionais=%d, a segunda promoção duplicou a contagem", got)
	}
}

// O prazo é medido pela REGRA, com relógio injetado.
func TestExpiredRespeitaOPrazo(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(4, 4, 5*time.Minute, c.agora)

	if _, err := r.Acquire("cedo", cfg(), KindPairing); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	c.avanca(4 * time.Minute)
	if got := r.Expired(); len(got) != 0 {
		t.Fatalf("expirou dentro do prazo: %v", got)
	}
	c.avanca(2 * time.Minute) // total 6min, além dos 5
	got := r.Expired()
	if len(got) != 1 || got[0] != "cedo" {
		t.Fatalf("Expired()=%v, quero [cedo]", got)
	}
}

// Sessão OPERACIONAL nunca expira por este prazo: ela não espera por ninguém.
func TestExpiredNaoTocaEmSessaoOperacional(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(4, 4, time.Minute, c.agora)
	if _, err := r.Acquire("op1", cfg(), KindOperational); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	c.avanca(time.Hour)
	if got := r.Expired(); len(got) != 0 {
		t.Fatalf("Expired()=%v: sessão operacional não espera por humano nenhum", got)
	}
}

// Expired RELATA, não age: parar um browser fala CDP e leva segundos, e fazê-lo
// sob o lock faria todo Acquire esperar por um desligamento alheio.
func TestExpiredNaoLiberaSozinho(t *testing.T) {
	c := novoRelogio()
	r := NewWithQuotas(4, 4, time.Minute, c.agora)
	if _, err := r.Acquire("p1", cfg(), KindPairing); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	c.avanca(2 * time.Minute)
	if got := r.Expired(); len(got) != 1 {
		t.Fatalf("Expired()=%v", got)
	}
	if !r.Holds("p1") {
		t.Fatal("Expired largou a sessão sozinho; quem para o browser decide quando")
	}
}
