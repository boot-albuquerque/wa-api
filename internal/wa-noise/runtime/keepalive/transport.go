// Package keepalive cuida do ping periodico que mantem o websocket vivo e do
// que fazer quando ele para de responder.
//
// O pacote nao toca socketLock e nao conhece o socket: as tres acoes de
// conexao de que ele precisa — desconectar, limpar a expectativa de desconexao
// e religar — atravessam a interface Transport como metodos ja' atomicos da
// raiz, que sao quem segura o lock. Ver PATCHES.md, "Fase F/G — lote 10".
package keepalive

import (
	"context"
	"time"

	waLog "wa-api/internal/wa-noise/observability/log"
	waBinary "wa-api/internal/wa-noise/protocol/binary"
)

// Timing sao os quatro parametros de tempo do keepalive. Continuam morando na
// raiz como variaveis exportadas (wa-noise.KeepAlive*), e por isso sao lidos
// atraves de Transport.Timing() **a cada iteracao** do loop, e nao capturados
// no comeco: um consumidor pode ajusta-los com o cliente ja' rodando, que era o
// comportamento antes da extracao.
type Timing struct {
	// ResponseDeadline e' quanto tempo esperar pela resposta de um ping.
	ResponseDeadline time.Duration
	// IntervalMin e IntervalMax delimitam o sorteio do intervalo entre pings.
	IntervalMin time.Duration
	IntervalMax time.Duration
	// MaxFailTime e' ha' quanto tempo sem sucesso o loop forca uma reconexao.
	MaxFailTime time.Duration
}

// Transport e' a fatia do cliente de que o keepalive precisa.
type Transport interface {
	// Log e' o logger do cliente.
	Log() waLog.Logger
	// Timing devolve os parametros de tempo correntes. Chamado a cada volta do
	// loop, de proposito — ver Timing.
	Timing() Timing
	// AutoReconnectEnabled e' Client.EnableAutoReconnect.
	AutoReconnectEnabled() bool
	// SendPing envia o <iq type="get" xmlns="w:p"> e devolve o canal por onde a
	// resposta chega. Corresponde ao sendIQAsync que o codigo original fazia.
	SendPing(ctx context.Context) (<-chan *waBinary.Node, error)
	// DispatchEvent entrega um evento aos handlers registrados.
	DispatchEvent(evt any)
	// Disconnect fecha o websocket. E' Client.Disconnect: ja' segura socketLock
	// por dentro.
	Disconnect()
	// ResetExpectedDisconnect desmarca a expectativa de desconexao, para que a
	// queda forcada pelo keepalive volte a disparar reconexao automatica.
	ResetExpectedDisconnect()
	// AutoReconnect e' o laco de religamento da raiz (Client.autoReconnect).
	AutoReconnect(ctx context.Context)
}
