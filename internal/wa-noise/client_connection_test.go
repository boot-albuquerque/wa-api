// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"go.mau.fi/util/exsync"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/socket"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
	waLog "wa-api/internal/wa-noise/observability/log"
)

// connTestClient e' o cliente minimo do lote 10. Difere de recvTestClient
// (lote 9) por trazer o que o caminho de conexao toca sem socket: o
// expectedDisconnect, o canal socketWait e o mapa de response waiters.
//
// Nao ha' socket: tudo que exige um websocket vivo esta' documentado como
// lacuna em PATCHES.md, nao simulado por duplo.
func connTestClient() *Client {
	ownID := types.NewJID("5511999999999", types.DefaultUserServer)
	return &Client{
		Log:                waLog.Noop,
		Store:              &store.Device{ID: &ownID},
		socketWait:         make(chan struct{}),
		expectedDisconnect: exsync.NewEvent(),
		responseWaiters:    make(map[string]chan<- *waBinary.Node),
	}
}

// --- isRetryableConnectError ---

// A classificacao decide se ConnectContext devolve erro ao chamador ou engole o
// erro e reconecta em background. Um 403 (banimento) tratado como transitorio
// faria o cliente insistir para sempre contra uma conta banida.
func TestIsRetryableConnectErrorStatusCodes(t *testing.T) {
	cases := []struct {
		code int
		want bool
	}{
		{408, true},
		{500, true},
		{501, true},
		{502, true},
		{503, true},
		{504, true},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, false},
		{505, false},
		{200, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			err := socket.ErrWithStatusCode{StatusCode: tc.code}
			if got := isRetryableConnectError(err); got != tc.want {
				t.Errorf("isRetryableConnectError(%d) = %v, queria %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestIsRetryableConnectErrorDialFailed(t *testing.T) {
	if !isRetryableConnectError(socket.ErrDialFailed) {
		t.Error("ErrDialFailed deveria ser retentavel")
	}
	if !isRetryableConnectError(fmt.Errorf("connect: %w", socket.ErrDialFailed)) {
		t.Error("ErrDialFailed embrulhado deveria ser retentavel")
	}
}

// Erro de rede (timeout, DNS, recusa de conexao) e' o caso classico de "tente
// de novo mais tarde".
func TestIsRetryableConnectErrorNetworkError(t *testing.T) {
	netErr := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	if !isRetryableConnectError(netErr) {
		t.Error("erro de rede deveria ser retentavel")
	}
}

// Um erro sem status e sem natureza de rede — por exemplo o handshake Noise
// falhando — nao e' transitorio: insistir so' repete a mesma falha.
func TestIsRetryableConnectErrorPlainError(t *testing.T) {
	if isRetryableConnectError(errors.New("noise handshake failed")) {
		t.Error("erro generico nao deveria ser retentavel")
	}
	if isRetryableConnectError(nil) {
		t.Error("nil nao deveria ser retentavel")
	}
}

// --- guardas de *Client nil ---

// Todos os metodos publicos de conexao sao chamados por codigo de aplicacao que
// pode ter um cliente nil na mao. Sao guardas explicitos no fork; se algum
// sumir, o processo cai.
func TestConnectionMethodsAreNilSafe(t *testing.T) {
	var cli *Client
	if cli.IsConnected() {
		t.Error("IsConnected() em cliente nil deveria ser false")
	}
	if cli.IsLoggedIn() {
		t.Error("IsLoggedIn() em cliente nil deveria ser false")
	}
	if cli.WaitForConnection(time.Millisecond) {
		t.Error("WaitForConnection() em cliente nil deveria ser false")
	}
	if err := cli.ConnectContext(t.Context()); !errors.Is(err, ErrClientIsNil) {
		t.Errorf("ConnectContext() = %v, queria ErrClientIsNil", err)
	}
	// Nao podem entrar em panic.
	cli.Disconnect()
	cli.ResetConnection()
}

// --- estado de conexao sem socket ---

func TestIsConnectedWithoutSocket(t *testing.T) {
	cli := connTestClient()
	if cli.IsConnected() {
		t.Error("cliente sem socket nao pode se dizer conectado")
	}
	if cli.IsLoggedIn() {
		t.Error("cliente novo nao pode se dizer logado")
	}
}

// WaitForConnection tem que desistir no prazo quando nunca ha' conexao, em vez
// de bloquear para sempre.
func TestWaitForConnectionTimesOut(t *testing.T) {
	cli := connTestClient()
	start := time.Now()
	if cli.WaitForConnection(20 * time.Millisecond) {
		t.Error("WaitForConnection deveria falhar sem socket")
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Errorf("voltou em %s, antes do timeout pedido", elapsed)
	}
}

// A outra saida de WaitForConnection: um disconnect esperado cancela a espera
// na hora, sem esperar o timeout.
func TestWaitForConnectionAbortsOnExpectedDisconnect(t *testing.T) {
	cli := connTestClient()
	cli.expectDisconnect()
	start := time.Now()
	if cli.WaitForConnection(10 * time.Second) {
		t.Error("WaitForConnection deveria falhar apos expectDisconnect")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("levou %s; deveria abortar imediatamente", elapsed)
	}
}

// --- canal de espera do socket ---

// closeSocketWaitChan acorda quem espera e ja' deixa um canal novo no lugar. Se
// nao trocasse o canal, a proxima conexao acordaria os esperadores na hora.
func TestCloseSocketWaitChanReplacesChannel(t *testing.T) {
	cli := connTestClient()
	before := cli.getSocketWaitChan()

	cli.closeSocketWaitChan()

	select {
	case <-before:
	default:
		t.Fatal("canal antigo deveria ter sido fechado")
	}
	after := cli.getSocketWaitChan()
	select {
	case <-after:
		t.Error("canal novo nao pode ja' vir fechado")
	default:
	}
}

// --- estado de disconnect esperado ---

func TestExpectAndResetDisconnect(t *testing.T) {
	cli := connTestClient()
	if cli.isExpectedDisconnect() {
		t.Error("cliente novo nao pode ja' esperar disconnect")
	}

	cli.forceAutoReconnect.Store(true)
	cli.expectDisconnect()
	if !cli.isExpectedDisconnect() {
		t.Error("expectDisconnect() nao marcou")
	}
	if cli.forceAutoReconnect.Load() {
		t.Error("expectDisconnect() tem que desarmar forceAutoReconnect")
	}

	cli.forceAutoReconnect.Store(true)
	cli.resetExpectedDisconnect()
	if cli.isExpectedDisconnect() {
		t.Error("resetExpectedDisconnect() nao limpou")
	}
	if cli.forceAutoReconnect.Load() {
		t.Error("resetExpectedDisconnect() tem que desarmar forceAutoReconnect")
	}
}

// autoReconnect sai na hora quando o auto-reconnect esta' desligado ou quando
// nao ha' device pareado — sem esses cortes ele giraria em vao.
func TestAutoReconnectStopsWhenDisabledOrUnpaired(t *testing.T) {
	cli := connTestClient()
	cli.EnableAutoReconnect = false
	done := make(chan struct{})
	go func() { cli.autoReconnect(t.Context()); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("autoReconnect nao voltou com EnableAutoReconnect=false")
	}

	cli = connTestClient()
	cli.EnableAutoReconnect = true
	cli.Store.ID = nil
	done = make(chan struct{})
	go func() { cli.autoReconnect(t.Context()); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("autoReconnect nao voltou sem device pareado")
	}
}
