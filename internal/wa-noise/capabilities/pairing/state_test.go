package pairing

import "testing"

// State.linking e' escrito por PairPhone (goroutine da aplicacao) e lido por
// HandleCodeNotification (goroutine do handler de notificacao). Como ponteiro
// comum isso era corrida de dados (F50). Este teste falha sob -race na versao
// antiga e passa com atomic.Pointer.
func TestStateLinkingSuportaLeituraEEscritaConcorrentes(t *testing.T) {
	var s State
	const rodadas = 200

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < rodadas; i++ {
			s.SetLinking(&LinkingCache{LinkingCode: "ABCD1234"})
			s.SetLinking(nil)
		}
	}()
	for i := 0; i < rodadas; i++ {
		// O valor alterna entre nil e preenchido de proposito: o que importa e'
		// que a leitura nunca enxergue um LinkingCache pela metade.
		if c := s.Linking(); c != nil && c.LinkingCode != "ABCD1234" {
			t.Fatalf("li um LinkingCache parcialmente publicado: %+v", c)
		}
	}
	<-done
}

func TestStateZeroValueTemLinkingNil(t *testing.T) {
	var s State
	if got := s.Linking(); got != nil {
		t.Errorf("Linking() = %+v, esperava nil no zero value", got)
	}
}
