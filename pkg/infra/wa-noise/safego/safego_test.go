package safego

import (
	"errors"
	"testing"
	"time"
)

// waitFor recebe de done ou falha o teste. Existe porque SafeGo é
// fire-and-forget: sem um ponto de sincronização o teste ou não observa nada
// (e passa por acidente) ou lê a variável compartilhada em corrida com a
// goroutine — foi o que o -race acusava na versão anterior deste arquivo.
func waitFor(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a goroutine de SafeGo não executou dentro do prazo")
	}
}

// TestSafeGo_NormalExec garante que a função entregue é de fato executada.
func TestSafeGo_NormalExec(t *testing.T) {
	done := make(chan struct{})
	SafeGo("test", func() { close(done) })
	waitFor(t, done)
}

// TestSafeGo_PanicRecovered garante que um panic dentro da goroutine é
// recuperado em vez de matar o processo: o recover vive na própria goroutine,
// portanto a prova é o binário de teste seguir vivo depois de fn ter rodado.
func TestSafeGo_PanicRecovered(t *testing.T) {
	done := make(chan struct{})
	SafeGo("test-panic", func() {
		close(done)
		panic(errors.New("boom"))
	})
	waitFor(t, done)
	// Dá à goroutine a chance de completar o defer/recover antes de o teste
	// terminar; um panic não recuperado derrubaria todo o processo de teste.
	time.Sleep(50 * time.Millisecond)
}
