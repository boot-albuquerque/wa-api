// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"testing"
	"time"
)

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

// O caso normal: o sorteio tem que ficar dentro da janela configurada, em toda
// tentativa. O jitter existe para os pings de varias sessoes nao baterem no
// servidor todos no mesmo instante.
func TestRandomKeepAliveIntervalWithinWindow(t *testing.T) {
	setKeepAliveWindow(t, 20*time.Second, 30*time.Second)
	for i := 0; i < 200; i++ {
		got := randomKeepAliveInterval()
		if got < KeepAliveIntervalMin || got >= KeepAliveIntervalMax {
			t.Fatalf("intervalo %v fora de [%v, %v)", got, KeepAliveIntervalMin, KeepAliveIntervalMax)
		}
	}
}

// Se todo sorteio devolvesse o mesmo valor nao haveria jitter nenhum.
func TestRandomKeepAliveIntervalVaries(t *testing.T) {
	setKeepAliveWindow(t, 20*time.Second, 30*time.Second)
	seen := make(map[time.Duration]struct{})
	for i := 0; i < 100; i++ {
		seen[randomKeepAliveInterval()] = struct{}{}
	}
	if len(seen) < 2 {
		t.Errorf("100 sorteios deram %d valor(es) distinto(s); nao ha' jitter", len(seen))
	}
}

// BUG DO LOTE 10: janela degenerada derrubava o processo.
//
// KeepAliveIntervalMin e KeepAliveIntervalMax sao variaveis *exportadas*.
// Configurar as duas com o mesmo valor — a forma obvia de pedir "pingue de 20
// em 20 segundos, sem jitter" — fazia `rand.Int64N(0)` entrar em panic dentro
// do keepAliveLoop, um goroutine sem recover: processo inteiro no chao por uma
// linha de configuracao aparentemente inocente.
func TestRandomKeepAliveIntervalEqualBoundsDoesNotPanic(t *testing.T) {
	setKeepAliveWindow(t, 20*time.Second, 20*time.Second)
	got := randomKeepAliveInterval()
	if got != 20*time.Second {
		t.Errorf("intervalo = %v, queria 20s (o proprio minimo)", got)
	}
}

// Mesma raiz, outra digitacao errada: pontas invertidas dariam argumento
// negativo, que tambem entra em panic.
func TestRandomKeepAliveIntervalInvertedBoundsDoesNotPanic(t *testing.T) {
	setKeepAliveWindow(t, 30*time.Second, 10*time.Second)
	got := randomKeepAliveInterval()
	if got != 30*time.Second {
		t.Errorf("intervalo = %v, queria 30s (o proprio minimo)", got)
	}
}

// Uma janela de 1ms ainda e' valida e nao pode virar o ramo degenerado.
func TestRandomKeepAliveIntervalMinimalWindow(t *testing.T) {
	setKeepAliveWindow(t, 20*time.Second, 20*time.Second+time.Millisecond)
	for i := 0; i < 20; i++ {
		if got := randomKeepAliveInterval(); got != 20*time.Second {
			t.Fatalf("intervalo = %v, queria 20s", got)
		}
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
