// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"math/rand/v2"
	"time"

	"wa-api/internal/wa-noise/types"
	"wa-api/internal/wa-noise/types/events"
)

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

// randomKeepAliveInterval sorteia o intervalo ate' o proximo ping dentro de
// [KeepAliveIntervalMin, KeepAliveIntervalMax).
//
// A guarda de janela nao-positiva e' o que impede um panic: `rand.Int64N`
// entra em panic com argumento <= 0, e as duas pontas sao variaveis
// *exportadas* do pacote. Configurar um intervalo fixo — KeepAliveIntervalMin
// == KeepAliveIntervalMax, o jeito obvio de pedir "pingue de 20 em 20s" — ou
// inverter as pontas por engano derrubava o processo inteiro, porque este
// sorteio roda num goroutine sem recover. Com a janela degenerada o intervalo
// passa a ser o proprio minimo, que e' o comportamento que quem configurou
// assim esperava.
func randomKeepAliveInterval() time.Duration {
	minMS := KeepAliveIntervalMin.Milliseconds()
	window := KeepAliveIntervalMax.Milliseconds() - minMS
	if window <= 0 {
		return time.Duration(minMS) * time.Millisecond
	}
	return time.Duration(rand.Int64N(window)+minMS) * time.Millisecond
}

func (cli *Client) keepAliveLoop(ctx, connCtx context.Context) {
	lastSuccess := time.Now()
	var errorCount int
	for {
		select {
		case <-time.After(randomKeepAliveInterval()):
			isSuccess, shouldContinue := cli.sendKeepAlive(connCtx)
			if !shouldContinue {
				return
			} else if !isSuccess {
				errorCount++
				go cli.dispatchEvent(&events.KeepAliveTimeout{
					ErrorCount:  errorCount,
					LastSuccess: lastSuccess,
				})
				if cli.EnableAutoReconnect && time.Since(lastSuccess) > KeepAliveMaxFailTime {
					cli.Log.Debugf("Forcing reconnect due to keepalive failure")
					cli.Disconnect()
					cli.resetExpectedDisconnect()
					go cli.autoReconnect(ctx)
				}
			} else {
				if errorCount > 0 {
					errorCount = 0
					go cli.dispatchEvent(&events.KeepAliveRestored{})
				}
				lastSuccess = time.Now()
			}
		case <-connCtx.Done():
			return
		}
	}
}

func (cli *Client) sendKeepAlive(ctx context.Context) (isSuccess, shouldContinue bool) {
	respCh, err := cli.sendIQAsync(ctx, infoQuery{
		Namespace: "w:p",
		Type:      "get",
		To:        types.ServerJID,
	})
	if ctx.Err() != nil {
		return false, false
	} else if err != nil {
		cli.Log.Warnf("Failed to send keepalive: %v", err)
		return false, true
	}
	select {
	case <-respCh:
		// All good
		return true, true
	case <-time.After(KeepAliveResponseDeadline):
		cli.Log.Warnf("Keepalive timed out")
		return false, true
	case <-ctx.Done():
		return false, false
	}
}
