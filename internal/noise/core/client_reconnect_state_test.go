package core

import (
	"testing"
	"time"
)

func TestLastSuccessfulConnectZeroQuandoNuncaConectou(t *testing.T) {
	var cli Client
	if got := cli.LastSuccessfulConnect(); !got.IsZero() {
		t.Errorf("= %v, esperava o zero de time.Time", got)
	}
	if got := cli.AutoReconnectErrors(); got != 0 {
		t.Errorf("AutoReconnectErrors() = %d, esperava 0", got)
	}
}

func TestLastSuccessfulConnectRoundTrip(t *testing.T) {
	var cli Client
	agora := time.Now()
	cli.lastSuccessfulConnectUnixNano.Store(agora.UnixNano())
	if got := cli.LastSuccessfulConnect(); !got.Equal(agora) {
		t.Errorf("= %v, esperava %v", got, agora)
	}
}

// Os dois campos sao escritos por handleConnectSuccess e por autoReconnect, em
// goroutines diferentes. Como int/time.Time comuns isso era data race (F46).
func TestContadoresDeReconexaoSuportamAcessoConcorrente(t *testing.T) {
	var cli Client
	const rodadas = 500

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < rodadas; i++ {
			cli.lastSuccessfulConnectUnixNano.Store(time.Now().UnixNano())
			cli.autoReconnectErrors.Store(0)
		}
	}()
	for i := 0; i < rodadas; i++ {
		_ = cli.LastSuccessfulConnect()
		cli.autoReconnectErrors.Add(1)
		_ = cli.AutoReconnectErrors()
	}
	<-done
}
