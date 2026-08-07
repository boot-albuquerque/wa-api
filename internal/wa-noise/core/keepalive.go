package whatsmeow

import (
	"context"
	"time"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/runtime/keepalive"
	"wa-api/internal/wa-noise/protocol/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// Estas quatro variaveis continuam sendo a fonte da verdade e continuam na raiz
// porque sao API exportada e ajustavel em tempo de execucao. O subpacote
// keepalive nao as duplica: le as quatro por keepAliveTransport.Timing(), que e'
// chamado a cada volta do loop.
var (
	// KeepAliveResponseDeadline specifies the duration to wait for a response to websocket keepalive pings.
	KeepAliveResponseDeadline = 10 * time.Second
	// KeepAliveIntervalMin specifies the minimum interval for websocket keepalive pings.
	KeepAliveIntervalMin = 20 * time.Second
	// KeepAliveIntervalMax specifies the maximum interval for websocket keepalive pings.
	KeepAliveIntervalMax = 30 * time.Second

	// KeepAliveMaxFailTime specifies the maximum time to wait before forcing a reconnect if keepalives fail repeatedly.
	KeepAliveMaxFailTime = 3 * time.Minute
)

// keepAliveTransport adapta *Client a keepalive.Transport. Existe para que o
// pacote internal/wa-noise/runtime/keepalive possa operar sobre uma interface estreita
// sem importar o pacote raiz (o que fecharia um ciclo).
//
// Ver ADR-0004 e PATCHES.md, "Fase F/G — lote 10".
type keepAliveTransport struct {
	cli *Client
}

var _ keepalive.Transport = keepAliveTransport{}

func (cli *Client) keepAliveT() keepalive.Transport {
	return keepAliveTransport{cli}
}

func (t keepAliveTransport) Log() waLog.Logger {
	return t.cli.Log
}

// Timing le as quatro variaveis exportadas na hora da chamada. E' o que
// preserva o comportamento anterior a extracao, em que cada iteracao do loop
// lia a variavel de novo.
func (t keepAliveTransport) Timing() keepalive.Timing {
	return keepalive.Timing{
		ResponseDeadline: KeepAliveResponseDeadline,
		IntervalMin:      KeepAliveIntervalMin,
		IntervalMax:      KeepAliveIntervalMax,
		MaxFailTime:      KeepAliveMaxFailTime,
	}
}

func (t keepAliveTransport) AutoReconnectEnabled() bool {
	return t.cli.EnableAutoReconnect
}

func (t keepAliveTransport) SendPing(ctx context.Context) (<-chan *waBinary.Node, error) {
	return t.cli.sendIQAsync(ctx, infoQuery{
		Namespace: "w:p",
		Type:      "get",
		To:        types.ServerJID,
	})
}

func (t keepAliveTransport) DispatchEvent(evt any) {
	t.cli.dispatchEvent(evt)
}

// Disconnect, ResetExpectedDisconnect e AutoReconnect sao os tres pontos em que
// o keepalive mexe no ciclo de vida da conexao. Os tres sao metodos da raiz que
// ja' encapsulam socketLock — o subpacote nunca ve o mutex, exatamente como a
// Fase D exigiu.
func (t keepAliveTransport) Disconnect() {
	t.cli.Disconnect()
}

func (t keepAliveTransport) ResetExpectedDisconnect() {
	t.cli.resetExpectedDisconnect()
}

func (t keepAliveTransport) AutoReconnect(ctx context.Context) {
	t.cli.autoReconnect(ctx)
}

func (cli *Client) keepAliveLoop(ctx, connCtx context.Context) {
	keepalive.Loop(ctx, connCtx, cli.keepAliveT())
}

func (cli *Client) sendKeepAlive(ctx context.Context) (isSuccess, shouldContinue bool) {
	return keepalive.Send(ctx, cli.keepAliveT())
}
