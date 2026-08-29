package keepalive

import (
	"context"
	"math/rand/v2"
	"time"

	"wa-api/internal/noise/protocol/types/events"
)

// RandomInterval sorteia o intervalo ate' o proximo ping dentro de
// [IntervalMin, IntervalMax).
//
// A guarda de janela nao-positiva e' o que impede um panic: `rand.Int64N`
// entra em panic com argumento <= 0, e as duas pontas sao variaveis
// *exportadas* do pacote raiz. Configurar um intervalo fixo — IntervalMin ==
// IntervalMax, o jeito obvio de pedir "pingue de 20 em 20s" — ou inverter as
// pontas por engano derrubava o processo inteiro, porque este sorteio roda num
// goroutine sem recover. Com a janela degenerada o intervalo passa a ser o
// proprio minimo, que e' o comportamento que quem configurou assim esperava.
func RandomInterval(t Timing) time.Duration {
	minMS := t.IntervalMin.Milliseconds()
	window := t.IntervalMax.Milliseconds() - minMS
	if window <= 0 {
		return time.Duration(minMS) * time.Millisecond
	}
	return time.Duration(rand.Int64N(window)+minMS) * time.Millisecond
}

// Loop mantem o websocket vivo mandando pings periodicos ate' connCtx acabar.
//
// ctx e connCtx sao mesmo dois contextos diferentes: connCtx e' o da conexao
// atual (morre quando o socket cai) e delimita o loop e o envio; ctx e' o de
// eventos do cliente e e' o que sobrevive para o religamento. Trocar um pelo
// outro faria a reconexao nascer ja' cancelada.
func Loop(ctx, connCtx context.Context, t Transport) {
	lastSuccess := time.Now()
	var errorCount int
	for {
		select {
		case <-time.After(RandomInterval(t.Timing())):
			isSuccess, shouldContinue := Send(connCtx, t)
			if !shouldContinue {
				return
			} else if !isSuccess {
				errorCount++
				go t.DispatchEvent(&events.KeepAliveTimeout{
					ErrorCount:  errorCount,
					LastSuccess: lastSuccess,
				})
				if t.AutoReconnectEnabled() && time.Since(lastSuccess) > t.Timing().MaxFailTime {
					t.Log().Debugf("Forcing reconnect due to keepalive failure")
					t.Disconnect()
					t.ResetExpectedDisconnect()
					go t.AutoReconnect(ctx)
				}
			} else {
				if errorCount > 0 {
					errorCount = 0
					go t.DispatchEvent(&events.KeepAliveRestored{})
				}
				lastSuccess = time.Now()
			}
		case <-connCtx.Done():
			return
		}
	}
}

// Send manda um ping e espera a resposta.
//
// Os dois booleanos nao sao redundantes: shouldContinue=false significa "a
// conexao acabou, encerre o loop" e so' acontece por contexto cancelado;
// isSuccess=false com shouldContinue=true e' uma falha contabilizavel (envio
// falhou ou resposta nao veio a tempo), que o loop conta e eventualmente
// converte em reconexao forcada.
func Send(ctx context.Context, t Transport) (isSuccess, shouldContinue bool) {
	respCh, err := t.SendPing(ctx)
	if ctx.Err() != nil {
		return false, false
	} else if err != nil {
		t.Log().Warnf("Failed to send keepalive: %v", err)
		return false, true
	}
	select {
	case <-respCh:
		// All good
		return true, true
	case <-time.After(t.Timing().ResponseDeadline):
		t.Log().Warnf("Keepalive timed out")
		return false, true
	case <-ctx.Done():
		return false, false
	}
}
