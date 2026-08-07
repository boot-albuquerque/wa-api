package wanoise

import (
	"context"
	"testing"
	"time"
)

// Os testes do sorteio de intervalo e do laco em si mudaram para
// internal/wa-noise/runtime/keepalive no lote 10 da Fase F/G, junto com o codigo. O que
// fica aqui e' a fiacao: que a fachada da raiz repassa as quatro variaveis
// exportadas e os tres metodos de ciclo de vida da conexao.

// setKeepAliveWindow troca as duas variaveis exportadas e as restaura no fim.
// Sao globais do pacote, entao estes testes nao podem rodar em paralelo.
func setKeepAliveWindow(t *testing.T, minD, maxD time.Duration) {
	t.Helper()
	oldMin, oldMax := KeepAliveIntervalMin, KeepAliveIntervalMax
	t.Cleanup(func() {
		KeepAliveIntervalMin, KeepAliveIntervalMax = oldMin, oldMax
	})
	KeepAliveIntervalMin, KeepAliveIntervalMax = minD, maxD
}

// Timing tem que refletir as quatro variaveis exportadas **no momento da
// chamada**. Se a fachada capturasse os valores uma vez so', ajustar
// KeepAliveIntervalMin com o cliente rodando deixaria de ter efeito — que era o
// comportamento antes da extracao.
func TestKeepAliveTransportTimingReadsExportedVars(t *testing.T) {
	setKeepAliveWindow(t, 7*time.Second, 9*time.Second)
	oldDeadline, oldMaxFail := KeepAliveResponseDeadline, KeepAliveMaxFailTime
	t.Cleanup(func() {
		KeepAliveResponseDeadline, KeepAliveMaxFailTime = oldDeadline, oldMaxFail
	})
	KeepAliveResponseDeadline = 3 * time.Second
	KeepAliveMaxFailTime = 11 * time.Second

	got := connTestClient().keepAliveT().Timing()

	if got.IntervalMin != 7*time.Second || got.IntervalMax != 9*time.Second {
		t.Errorf("janela = [%v, %v), queria [7s, 9s)", got.IntervalMin, got.IntervalMax)
	}
	if got.ResponseDeadline != 3*time.Second {
		t.Errorf("ResponseDeadline = %v, queria 3s", got.ResponseDeadline)
	}
	if got.MaxFailTime != 11*time.Second {
		t.Errorf("MaxFailTime = %v, queria 11s", got.MaxFailTime)
	}
}

// AutoReconnectEnabled tem que espelhar o campo do cliente, nao um valor fixo.
func TestKeepAliveTransportAutoReconnectEnabled(t *testing.T) {
	cli := connTestClient()
	cli.EnableAutoReconnect = false
	if cli.keepAliveT().AutoReconnectEnabled() {
		t.Error("AutoReconnectEnabled = true com EnableAutoReconnect = false")
	}
	cli.EnableAutoReconnect = true
	if !cli.keepAliveT().AutoReconnectEnabled() {
		t.Error("AutoReconnectEnabled = false com EnableAutoReconnect = true")
	}
}

// keepAliveLoop tem que voltar quando o contexto da conexao morre, senao cada
// reconexao deixaria um goroutine pingando um socket morto para tras.
func TestKeepAliveLoopStopsOnConnCtxDone(t *testing.T) {
	setKeepAliveWindow(t, time.Millisecond, 2*time.Millisecond)
	cli := connTestClient()
	connCtx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() { cli.keepAliveLoop(t.Context(), connCtx); close(done) }()
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("keepAliveLoop nao voltou apos o cancelamento do contexto da conexao")
	}
}

// sendKeepAlive com o contexto ja' cancelado tem que pedir a parada do loop
// (shouldContinue=false) em vez de contar como falha de keepalive.
func TestSendKeepAliveStopsOnCancelledContext(t *testing.T) {
	cli := connTestClient()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	isSuccess, shouldContinue := cli.sendKeepAlive(ctx)

	if isSuccess {
		t.Error("nao pode reportar sucesso com contexto cancelado")
	}
	if shouldContinue {
		t.Error("contexto cancelado tem que encerrar o loop, nao contar como falha")
	}
}

// sendKeepAlive sobre um cliente sem socket tem que devolver falha
// contabilizavel — o loop continua e tenta de novo — e nao encerrar o loop.
func TestSendKeepAliveOnDisconnectedClientCountsAsFailure(t *testing.T) {
	cli := connTestClient()

	isSuccess, shouldContinue := cli.sendKeepAlive(t.Context())

	if isSuccess {
		t.Error("cliente sem socket nao pode reportar sucesso")
	}
	if !shouldContinue {
		t.Error("falha de envio nao pode encerrar o loop")
	}
}
