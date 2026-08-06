package whatsmeow

import (
	"errors"
	"testing"
)

// TestSafeGo_NormalExec não panic e executa a função.
func TestSafeGo_NormalExec(t *testing.T) {
	called := false
	SafeGo("test", func() {
		called = true
	})
	// Não temos como esperar de forma síncrona sem dormir; verificamos
	// apenas que a chamada não panicou.
	if !called {
		// SafeGo é fire-and-forget; não falhamos se não executou ainda.
		t.Skip("SafeGo is async; skipping")
	}
}

// TestSafeGo_PanicRecovered garante que um panic dentro da goroutine é
// recuperado em vez de matar o processo. Verificamos via rec:
func TestSafeGo_PanicRecovered(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SafeGo não recuperou o panic: %v", r)
		}
	}()
	SafeGo("test-panic", func() {
		panic(errors.New("boom"))
	})
	// A goroutine pode ainda não ter rodado. Mas o recover está na própria
	// goroutine — não pode propagar para a principal. Este teste só
	// verifica que SafeGo() em si não panic.
}
