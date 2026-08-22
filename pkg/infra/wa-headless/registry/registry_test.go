package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	waheadless "wa-api/internal/wa-headless"
)

// cfg é uma configuração que NUNCA arranca: o Holder é preguiçoso e só sobe
// browser no primeiro Session(). Estes testes exercitam a contabilidade de
// slots, e um browser de verdade aqui mediria o Chromium, não o registry.
func cfg() waheadless.StartConfig {
	return waheadless.StartConfig{BinaryPath: "/nonexistent", ProfileDir: "/tmp/nao-usado"}
}

// TestAcquireRecusaAcimaDoTetoEmVezDeEsperar é a asserção central do desenho.
//
// A F86 mediu o custo de bloquear o chamador num limitador: a aquisição segurava
// o handler de eventos por 3,1s contra 121ms sem ela. Aqui o chamador é um
// handler HTTP, então esperar transformaria "esta máquina está cheia" em "este
// pedido pendurou" — pior, e muito mais difícil de ver. O teste mede o TEMPO
// justamente para que uma futura versão que espere não passe em silêncio.
func TestAcquireRecusaAcimaDoTetoEmVezDeEsperar(t *testing.T) {
	r := New(2)
	for _, id := range []string{"a", "b"} {
		if _, err := r.Acquire(id, cfg()); err != nil {
			t.Fatalf("Acquire(%q): %v", id, err)
		}
	}

	start := time.Now()
	_, err := r.Acquire("c", cfg())
	elapsed := time.Since(start)

	if !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("Acquire acima do teto devolveu %v, quero ErrAtCapacity", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("a recusa levou %v: o teto está ESPERANDO por um slot, e o "+
			"chamador é um handler HTTP", elapsed)
	}
	if r.Len() != 2 {
		t.Fatalf("Len=%d depois da recusa, quero 2 — a recusa não pode consumir slot", r.Len())
	}
}

// Uma sessão que este processo JÁ detém é devolvida mesmo com o pool cheio: o
// teto limita quantas existem, não quantas vezes cada uma é usada. Recusar aqui
// quebraria tráfego que funciona para proteger contra um custo já pago.
func TestAcquireDevolveSessaoExistenteMesmoNoTeto(t *testing.T) {
	r := New(1)
	first, err := r.Acquire("a", cfg())
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	again, err := r.Acquire("a", cfg())
	if err != nil {
		t.Fatalf("Acquire de sessão existente recusado com o pool cheio: %v", err)
	}
	if first != again {
		t.Fatal("Acquire criou um SEGUNDO holder para o mesmo txtID; invariante 13 " +
			"é um perfil, um dono ativo")
	}
}

// Release devolve o slot, senão o teto vira uma contagem que só sobe.
func TestReleaseDevolveOSlot(t *testing.T) {
	r := New(1)
	if _, err := r.Acquire("a", cfg()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := r.Acquire("b", cfg()); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("esperava teto, veio %v", err)
	}
	if _, err := r.Release(context.Background(), "a"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if r.Len() != 0 {
		t.Fatalf("Len=%d depois do Release, quero 0", r.Len())
	}
	if _, err := r.Acquire("b", cfg()); err != nil {
		t.Fatalf("o slot não foi devolvido: %v", err)
	}
}

// Release de sessão que este processo não detém é ERRO, e não um silêncio
// simpático: quem chamou acha que largou algo, e o que largou era de outro.
func TestReleaseDeSessaoDesconhecidaEErro(t *testing.T) {
	r := New(2)
	if _, err := r.Release(context.Background(), "fantasma"); !errors.Is(err, ErrUnknownSession) {
		t.Fatalf("got %v, want ErrUnknownSession", err)
	}
}

// Holds responde POSSE, e não prontidão. A ADR-0005 D6 separa as duas, e uma
// sonda que as confunde mente.
func TestHoldsRespondePosseENaoProntidao(t *testing.T) {
	r := New(2)
	if r.Holds("a") {
		t.Fatal("Holds disse sim antes de qualquer Acquire")
	}
	if _, err := r.Acquire("a", cfg()); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// Nenhum browser subiu — o Holder é preguiçoso. Holds continua verdadeiro,
	// que é a resposta certa: posse não é o mesmo que estar de pé.
	if !r.Holds("a") {
		t.Fatal("Holds negou uma sessão que este processo detém")
	}
}

// Teto zero ou negativo cai no padrão em vez de significar "ilimitado": um zero
// acidental removeria a proteção em silêncio, que é o modo de falha que um
// padrão existe para impedir.
func TestTetoZeroCaiNoPadraoENaoEmIlimitado(t *testing.T) {
	for _, max := range []int{0, -1} {
		r := New(max)
		for i := 0; i < DefaultMaxSessions; i++ {
			if _, err := r.Acquire(fmt.Sprintf("s%d", i), cfg()); err != nil {
				t.Fatalf("max=%d, Acquire %d: %v", max, i, err)
			}
		}
		if _, err := r.Acquire("excedente", cfg()); !errors.Is(err, ErrAtCapacity) {
			t.Fatalf("max=%d virou ILIMITADO: %v", max, err)
		}
	}
}

// O teto tem de valer sob concorrência, senão ele é uma sugestão: N goroutines
// pedindo txtIDs distintos ao mesmo tempo não podem passar do máximo.
func TestOTetoValeSobConcorrencia(t *testing.T) {
	const max = 3
	const goroutines = 40
	r := New(max)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var granted int
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := r.Acquire(fmt.Sprintf("s%d", i), cfg()); err == nil {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if granted != max {
		t.Fatalf("%d aquisições concedidas com teto %d", granted, max)
	}
	if r.Len() != max {
		t.Fatalf("Len=%d, quero %d", r.Len(), max)
	}
}

// txtID vazio é recusado: um mapa com chave "" aceitaria a primeira sessão sem
// dono e devolveria essa mesma para qualquer chamador que também esquecesse o id.
func TestTxtIDVazioERecusado(t *testing.T) {
	r := New(2)
	if _, err := r.Acquire("", cfg()); err == nil {
		t.Fatal("txtID vazio foi aceito")
	}
	if r.Len() != 0 {
		t.Fatalf("Len=%d, a recusa consumiu slot", r.Len())
	}
}
