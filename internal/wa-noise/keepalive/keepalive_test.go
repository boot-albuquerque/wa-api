// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package keepalive

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/types/events"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// fakeTransport e' o dublê de Transport. Todos os contadores sao protegidos por
// mutex porque Loop dispara DispatchEvent e AutoReconnect em goroutines — e o
// `-race` do make check pega qualquer descuido aqui.
type fakeTransport struct {
	mu sync.Mutex

	timing Timing
	// pingErr, quando != nil, faz SendPing falhar.
	pingErr error
	// replyDelay e' quanto tempo o dublê demora para responder o ping. Maior que
	// ResponseDeadline simula timeout.
	replyDelay time.Duration
	// neverReply faz o canal de resposta nunca receber nada.
	neverReply bool

	autoReconnectEnabled bool

	pings            int
	events           []any
	disconnects      int
	resets           int
	autoReconnects   int
	autoReconnectCtx context.Context
}

func newFake() *fakeTransport {
	return &fakeTransport{
		timing: Timing{
			ResponseDeadline: 50 * time.Millisecond,
			IntervalMin:      time.Millisecond,
			IntervalMax:      2 * time.Millisecond,
			MaxFailTime:      time.Hour,
		},
	}
}

func (f *fakeTransport) Log() waLog.Logger { return waLog.Noop }

func (f *fakeTransport) Timing() Timing {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.timing
}

func (f *fakeTransport) AutoReconnectEnabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.autoReconnectEnabled
}

func (f *fakeTransport) SendPing(ctx context.Context) (<-chan *waBinary.Node, error) {
	f.mu.Lock()
	f.pings++
	err, delay, never := f.pingErr, f.replyDelay, f.neverReply
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ch := make(chan *waBinary.Node, 1)
	if never {
		return ch, nil
	}
	if delay == 0 {
		ch <- &waBinary.Node{Tag: "iq"}
		return ch, nil
	}
	go func() {
		select {
		case <-time.After(delay):
			ch <- &waBinary.Node{Tag: "iq"}
		case <-ctx.Done():
		}
	}()
	return ch, nil
}

func (f *fakeTransport) DispatchEvent(evt any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evt)
}

func (f *fakeTransport) Disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disconnects++
}

func (f *fakeTransport) ResetExpectedDisconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resets++
}

func (f *fakeTransport) AutoReconnect(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.autoReconnects++
	f.autoReconnectCtx = ctx
}

func (f *fakeTransport) counts() (pings, disconnects, resets, reconnects int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pings, f.disconnects, f.resets, f.autoReconnects
}

// eventsOfType conta quantos eventos do tipo T foram despachados.
func eventsOfType[T any](f *fakeTransport) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, e := range f.events {
		if _, ok := e.(T); ok {
			n++
		}
	}
	return n
}

// waitFor espera cond virar verdadeira. Os testes de Loop sao inerentemente
// assincronos; um sleep fixo os deixaria lentos e instaveis.
func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

var _ Transport = (*fakeTransport)(nil)

// --- RandomInterval ---

// O caso normal: o sorteio tem que ficar dentro da janela configurada, em toda
// tentativa. O jitter existe para os pings de varias sessoes nao baterem no
// servidor todos no mesmo instante.
func TestRandomIntervalWithinWindow(t *testing.T) {
	timing := Timing{IntervalMin: 20 * time.Second, IntervalMax: 30 * time.Second}
	for i := 0; i < 200; i++ {
		got := RandomInterval(timing)
		if got < timing.IntervalMin || got >= timing.IntervalMax {
			t.Fatalf("intervalo %v fora de [%v, %v)", got, timing.IntervalMin, timing.IntervalMax)
		}
	}
}

// Se todo sorteio devolvesse o mesmo valor nao haveria jitter nenhum.
func TestRandomIntervalVaries(t *testing.T) {
	timing := Timing{IntervalMin: 20 * time.Second, IntervalMax: 30 * time.Second}
	seen := make(map[time.Duration]struct{})
	for i := 0; i < 100; i++ {
		seen[RandomInterval(timing)] = struct{}{}
	}
	if len(seen) < 2 {
		t.Errorf("100 sorteios deram %d valor(es) distinto(s); nao ha' jitter", len(seen))
	}
}

// BUG DO LOTE 10 (Fase E): janela degenerada derrubava o processo.
//
// As duas pontas sao variaveis *exportadas* da raiz. Configurar as duas com o
// mesmo valor — a forma obvia de pedir "pingue de 20 em 20 segundos, sem
// jitter" — fazia `rand.Int64N(0)` entrar em panic dentro do Loop, um goroutine
// sem recover: processo inteiro no chao por uma linha de configuracao
// aparentemente inocente.
func TestRandomIntervalEqualBoundsDoesNotPanic(t *testing.T) {
	got := RandomInterval(Timing{IntervalMin: 20 * time.Second, IntervalMax: 20 * time.Second})
	if got != 20*time.Second {
		t.Errorf("intervalo = %v, queria 20s (o proprio minimo)", got)
	}
}

// Mesma raiz, outra digitacao errada: pontas invertidas dariam argumento
// negativo, que tambem entra em panic.
func TestRandomIntervalInvertedBoundsDoesNotPanic(t *testing.T) {
	got := RandomInterval(Timing{IntervalMin: 30 * time.Second, IntervalMax: 10 * time.Second})
	if got != 30*time.Second {
		t.Errorf("intervalo = %v, queria 30s (o proprio minimo)", got)
	}
}

// Uma janela de 1ms ainda e' valida e nao pode virar o ramo degenerado.
func TestRandomIntervalMinimalWindow(t *testing.T) {
	timing := Timing{IntervalMin: 20 * time.Second, IntervalMax: 20*time.Second + time.Millisecond}
	for i := 0; i < 20; i++ {
		if got := RandomInterval(timing); got != 20*time.Second {
			t.Fatalf("intervalo = %v, queria 20s", got)
		}
	}
}

// --- Send ---

func TestSendSucceedsOnReply(t *testing.T) {
	f := newFake()
	isSuccess, shouldContinue := Send(context.Background(), f)
	if !isSuccess || !shouldContinue {
		t.Errorf("Send = (%v, %v), queria (true, true)", isSuccess, shouldContinue)
	}
}

// Falha de envio e' contabilizavel, nao fatal: o loop tem que continuar e
// tentar de novo.
func TestSendCountsFailureButContinues(t *testing.T) {
	f := newFake()
	f.pingErr = errors.New("socket fechado")

	isSuccess, shouldContinue := Send(context.Background(), f)

	if isSuccess {
		t.Error("envio que falhou nao pode reportar sucesso")
	}
	if !shouldContinue {
		t.Error("falha de envio nao pode encerrar o loop")
	}
}

// Resposta que nao chega dentro de ResponseDeadline conta como falha, mas
// tambem nao encerra o loop.
func TestSendTimesOutButContinues(t *testing.T) {
	f := newFake()
	f.neverReply = true

	isSuccess, shouldContinue := Send(context.Background(), f)

	if isSuccess {
		t.Error("timeout nao pode reportar sucesso")
	}
	if !shouldContinue {
		t.Error("timeout nao pode encerrar o loop")
	}
}

// Contexto ja' cancelado tem que pedir a parada do loop (shouldContinue=false)
// em vez de contar como falha de keepalive. O teste e' feito **antes** de olhar
// o erro de envio, que e' a ordem do codigo.
func TestSendStopsOnCancelledContext(t *testing.T) {
	f := newFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	isSuccess, shouldContinue := Send(ctx, f)

	if isSuccess {
		t.Error("nao pode reportar sucesso com contexto cancelado")
	}
	if shouldContinue {
		t.Error("contexto cancelado tem que encerrar o loop, nao contar como falha")
	}
}

// Cancelamento no meio da espera pela resposta tambem encerra o loop.
func TestSendStopsWhenContextCancelledWhileWaiting(t *testing.T) {
	f := newFake()
	f.neverReply = true
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	isSuccess, shouldContinue := Send(ctx, f)

	if isSuccess || shouldContinue {
		t.Errorf("Send = (%v, %v), queria (false, false)", isSuccess, shouldContinue)
	}
}

// --- Loop ---

// O loop tem que voltar quando o contexto da conexao morre, senao cada
// reconexao deixaria um goroutine pingando um socket morto para tras.
func TestLoopStopsOnConnCtxDone(t *testing.T) {
	f := newFake()
	connCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Loop(context.Background(), connCtx, f); close(done) }()
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Loop nao voltou apos o cancelamento do contexto da conexao")
	}
}

func TestLoopPingsRepeatedly(t *testing.T) {
	f := newFake()
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(context.Background(), connCtx, f)

	if !waitFor(t, func() bool { p, _, _, _ := f.counts(); return p >= 3 }) {
		t.Fatal("Loop nao repetiu os pings")
	}
}

// Falha de keepalive emite KeepAliveTimeout com contagem crescente.
func TestLoopEmitsKeepAliveTimeout(t *testing.T) {
	f := newFake()
	f.pingErr = errors.New("socket fechado")
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(context.Background(), connCtx, f)

	if !waitFor(t, func() bool { return eventsOfType[*events.KeepAliveTimeout](f) >= 2 }) {
		t.Fatal("Loop nao emitiu KeepAliveTimeout")
	}
}

// Depois de uma falha, o primeiro sucesso emite KeepAliveRestored — e so' o
// primeiro: se emitisse a cada sucesso, o consumidor receberia um "restaurado"
// por ping.
func TestLoopEmitsRestoredOnceAfterRecovery(t *testing.T) {
	f := newFake()
	f.pingErr = errors.New("socket fechado")
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(context.Background(), connCtx, f)

	if !waitFor(t, func() bool { return eventsOfType[*events.KeepAliveTimeout](f) >= 1 }) {
		t.Fatal("Loop nao emitiu KeepAliveTimeout")
	}
	f.mu.Lock()
	f.pingErr = nil
	f.mu.Unlock()

	if !waitFor(t, func() bool { return eventsOfType[*events.KeepAliveRestored](f) == 1 }) {
		t.Fatal("Loop nao emitiu KeepAliveRestored apos a recuperacao")
	}
	// Varios sucessos depois, ainda tem que ser exatamente um.
	if !waitFor(t, func() bool { p, _, _, _ := f.counts(); return p >= 10 }) {
		t.Fatal("Loop parou de pingar")
	}
	if got := eventsOfType[*events.KeepAliveRestored](f); got != 1 {
		t.Errorf("KeepAliveRestored emitido %d vezes, queria 1", got)
	}
}

// Falha prolongada com auto-reconnect ligado forca a reconexao: Disconnect,
// ResetExpectedDisconnect e AutoReconnect, nessa ordem. Sem o reset, a
// desconexao forcada aqui seria lida como "esperada" e o religamento nunca
// aconteceria.
func TestLoopForcesReconnectAfterMaxFailTime(t *testing.T) {
	f := newFake()
	f.pingErr = errors.New("socket fechado")
	f.autoReconnectEnabled = true
	f.timing.MaxFailTime = 0 // qualquer falha ja' passa do limite

	evtCtx := context.WithValue(context.Background(), struct{ k string }{"evt"}, true)
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(evtCtx, connCtx, f)

	if !waitFor(t, func() bool { _, d, r, ar := f.counts(); return d >= 1 && r >= 1 && ar >= 1 }) {
		_, d, r, ar := f.counts()
		t.Fatalf("reconexao forcada nao aconteceu: disconnects=%d resets=%d autoReconnects=%d", d, r, ar)
	}

	// AutoReconnect tem que receber o contexto de eventos (ctx), nao o da
	// conexao (connCtx) — que esta' morrendo justamente agora. Trocar os dois
	// faria a reconexao nascer ja' cancelada.
	f.mu.Lock()
	gotCtx := f.autoReconnectCtx
	f.mu.Unlock()
	if gotCtx != evtCtx {
		t.Error("AutoReconnect recebeu o contexto errado; tem que ser o de eventos, nao o da conexao")
	}
}

// Com auto-reconnect desligado, a falha prolongada emite o evento mas nao
// derruba a conexao por conta propria.
func TestLoopDoesNotReconnectWhenDisabled(t *testing.T) {
	f := newFake()
	f.pingErr = errors.New("socket fechado")
	f.autoReconnectEnabled = false
	f.timing.MaxFailTime = 0

	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(context.Background(), connCtx, f)

	if !waitFor(t, func() bool { return eventsOfType[*events.KeepAliveTimeout](f) >= 3 }) {
		t.Fatal("Loop nao emitiu KeepAliveTimeout")
	}
	if _, d, r, ar := f.counts(); d != 0 || r != 0 || ar != 0 {
		t.Errorf("com auto-reconnect desligado nao pode desconectar nem religar: %d/%d/%d", d, r, ar)
	}
}

// Timing e' relido a cada volta: um consumidor pode ajustar as variaveis
// exportadas com o cliente ja' rodando.
func TestLoopRereadsTimingEachIteration(t *testing.T) {
	f := newFake()
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Loop(context.Background(), connCtx, f)

	if !waitFor(t, func() bool { p, _, _, _ := f.counts(); return p >= 2 }) {
		t.Fatal("Loop nao comecou a pingar")
	}
	f.mu.Lock()
	f.timing.IntervalMin = time.Hour
	f.timing.IntervalMax = 2 * time.Hour
	f.mu.Unlock()

	p1, _, _, _ := f.counts()
	time.Sleep(50 * time.Millisecond)
	p2, _, _, _ := f.counts()
	// Depois da troca no maximo mais um ping pode sair (o sorteio ja' em curso).
	if p2 > p1+1 {
		t.Errorf("pings continuaram no ritmo antigo (%d -> %d); Timing nao foi relido", p1, p2)
	}
}
