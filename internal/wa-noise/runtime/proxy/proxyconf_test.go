// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package proxyconf

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// plainDialer implementa proxy.Dialer e **nao** implementa proxy.ContextDialer.
// E' o caso que fazia a type assertion crua entrar em panic.
type plainDialer struct{ called *int }

func (d plainDialer) Dial(network, addr string) (net.Conn, error) {
	*d.called++
	return nil, errors.New("plainDialer nao disca de verdade")
}

// ctxDialer implementa as duas interfaces.
type ctxDialer struct {
	dialCalled    *int
	ctxDialCalled *int
}

func (d ctxDialer) Dial(network, addr string) (net.Conn, error) {
	*d.dialCalled++
	return nil, errors.New("ctxDialer nao disca de verdade")
}

func (d ctxDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	*d.ctxDialCalled++
	return nil, errors.New("ctxDialer nao disca de verdade")
}

func newClients() Clients {
	return Clients{
		PreLogin:  &http.Client{},
		Websocket: &http.Client{},
		Media:     &http.Client{},
	}
}

// --- ContextDialerFor ---

// BUG DO LOTE 10 (Fase E): um Dialer sem DialContext derrubava o processo.
//
// Client.SetSOCKSProxy e' API exportada e aceita qualquer proxy.Dialer. A
// assertion `px.(proxy.ContextDialer)` entrava em panic para qualquer dialer
// customizado — e proxy.Dialer, a interface do parametro, nao exige
// DialContext.
func TestContextDialerForNonContextDialerDoesNotPanic(t *testing.T) {
	var called int
	dial := ContextDialerFor(plainDialer{called: &called})
	if dial == nil {
		t.Fatal("adaptador nao pode ser nil; o trafego sairia sem proxy")
	}

	_, _ = dial(context.Background(), "tcp", "example.com:443")

	// O proxy tem que ser realmente usado: um adaptador que ignorasse o dialer
	// vazaria o endereco real do cliente.
	if called != 1 {
		t.Errorf("Dial do proxy chamado %d vezes, queria 1", called)
	}
}

// Quando o dialer implementa ContextDialer, e' o DialContext dele que tem que
// ser usado — o adaptador nao pode descartar o contexto a toa.
func TestContextDialerForPrefersContextDialer(t *testing.T) {
	var dialCalled, ctxDialCalled int

	dial := ContextDialerFor(ctxDialer{dialCalled: &dialCalled, ctxDialCalled: &ctxDialCalled})
	_, _ = dial(context.Background(), "tcp", "example.com:443")

	if ctxDialCalled != 1 {
		t.Errorf("DialContext chamado %d vezes, queria 1", ctxDialCalled)
	}
	if dialCalled != 0 {
		t.Errorf("Dial sem contexto chamado %d vezes, queria 0", dialCalled)
	}
}

// --- TransportForProxy / TransportForDialer ---

// Cada chamada tem que devolver um transport novo. Reaproveitar
// http.DefaultTransport faria a configuracao de uma sessao vazar para todo o
// processo.
func TestTransportForProxyDoesNotShareDefaultTransport(t *testing.T) {
	a := TransportForProxy(nil)
	b := TransportForProxy(nil)
	if a == b {
		t.Error("duas chamadas devolveram o mesmo transport")
	}
	if a == http.DefaultTransport {
		t.Error("o transport padrao do processo foi devolvido em vez de um clone")
	}
	if a.Proxy != nil {
		t.Error("proxy nil tem que zerar Transport.Proxy, senao a leitura de https_proxy continua valendo")
	}
}

func TestTransportForProxyInstallsFunction(t *testing.T) {
	target, _ := url.Parse("http://proxy.local:8080")
	tr := TransportForProxy(http.ProxyURL(target))
	if tr.Proxy == nil {
		t.Fatal("Proxy nao foi instalado")
	}
	got, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "http", Host: "example.com"}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.String() != target.String() {
		t.Errorf("proxy = %v, queria %v", got, target)
	}
}

func TestTransportForDialerInstallsDialContext(t *testing.T) {
	var called int
	tr := TransportForDialer(plainDialer{called: &called})
	if tr.DialContext == nil {
		t.Fatal("DialContext nao foi instalado; o trafego sairia sem proxy")
	}
	_, _ = tr.DialContext(context.Background(), "tcp", "example.com:443")
	if called != 1 {
		t.Errorf("Dial do proxy chamado %d vezes, queria 1", called)
	}
}

// --- TransportForAddress ---

func TestTransportForAddressSchemes(t *testing.T) {
	cases := []struct {
		name        string
		addr        string
		wantErr     string
		wantProxy   bool
		wantDialCtx bool
	}{
		{name: "http", addr: "http://proxy.local:8080", wantProxy: true},
		{name: "https", addr: "https://proxy.local:8443", wantProxy: true},
		{name: "socks5", addr: "socks5://proxy.local:1080", wantDialCtx: true},
		{name: "socks5 com credenciais", addr: "socks5://u:p@proxy.local:1080", wantDialCtx: true},
		{name: "esquema desconhecido", addr: "ftp://proxy.local", wantErr: "unsupported proxy scheme"},
		{name: "url invalida", addr: "://sem-esquema", wantErr: "missing protocol scheme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := TransportForAddress(tc.addr)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, queria conter %q", err, tc.wantErr)
				}
				if tr != nil {
					t.Error("nenhum transport pode sair de um endereco invalido")
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, queria nil", err)
			}
			if tc.wantProxy && tr.Proxy == nil {
				t.Error("Proxy nao foi instalado")
			}
			if tc.wantDialCtx && tr.DialContext == nil {
				t.Error("DialContext nao foi instalado")
			}
		})
	}
}

// Endereco vazio zera o proxy em vez de virar erro de parse — e' como se
// desliga a leitura de https_proxy do ambiente.
func TestTransportForAddressEmptyClearsProxy(t *testing.T) {
	tr, err := TransportForAddress("")
	if err != nil {
		t.Fatalf("err = %v, queria nil", err)
	}
	if tr.Proxy != nil {
		t.Error("endereco vazio tem que deixar Proxy nil")
	}
}

// --- Apply ---

// Cada flag desliga exatamente um destino. Errar aqui manda trafego pelo canal
// errado — por exemplo o websocket por um proxy que so' deveria servir midia.
func TestApplySelectsTargets(t *testing.T) {
	cases := []struct {
		name                            string
		opt                             Options
		wantPreLogin, wantWS, wantMedia bool
	}{
		{name: "padrao", opt: Options{}, wantPreLogin: true, wantWS: true, wantMedia: true},
		{name: "NoWebsocket", opt: Options{NoWebsocket: true}, wantMedia: true},
		{name: "OnlyLogin", opt: Options{OnlyLogin: true}, wantPreLogin: true, wantMedia: true},
		{name: "NoMedia", opt: Options{NoMedia: true}, wantPreLogin: true, wantWS: true},
		{name: "tudo desligado", opt: Options{NoWebsocket: true, NoMedia: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClients()
			tr := TransportForProxy(nil)

			Apply(c, tr, tc.opt)

			if got := c.PreLogin.Transport != nil; got != tc.wantPreLogin {
				t.Errorf("PreLogin configurado = %v, queria %v", got, tc.wantPreLogin)
			}
			if got := c.Websocket.Transport != nil; got != tc.wantWS {
				t.Errorf("Websocket configurado = %v, queria %v", got, tc.wantWS)
			}
			if got := c.Media.Transport != nil; got != tc.wantMedia {
				t.Errorf("Media configurado = %v, queria %v", got, tc.wantMedia)
			}
		})
	}
}

// Os tres destinos recebem o **mesmo** transport, nao copias: e' o que faz o
// pool de conexoes ser compartilhado, como era antes da extracao.
func TestApplySharesOneTransport(t *testing.T) {
	c := newClients()
	tr := TransportForProxy(nil)

	Apply(c, tr, Options{})

	if c.PreLogin.Transport != tr || c.Websocket.Transport != tr || c.Media.Transport != tr {
		t.Error("os tres clientes tem que apontar para o mesmo transport")
	}
}

// Apply escreve no campo Transport; nunca troca os ponteiros de http.Client.
// Trocar quebraria SetMediaHTTPClient, que guarda o ponteiro do chamador.
func TestApplyKeepsClientPointers(t *testing.T) {
	c := newClients()
	preLogin, ws, media := c.PreLogin, c.Websocket, c.Media

	Apply(c, TransportForProxy(nil), Options{})

	if c.PreLogin != preLogin || c.Websocket != ws || c.Media != media {
		t.Error("Apply trocou um ponteiro de http.Client")
	}
}

// Os parametros do dialer SOCKS5 precisam ser positivos: timeout zero e'
// "sem limite", que trava a conexao para sempre se o proxy nao responder.
func TestSOCKSDialerConstantsArePositive(t *testing.T) {
	if SOCKSDialTimeout <= 0 {
		t.Errorf("SOCKSDialTimeout = %v, tem que ser positivo", SOCKSDialTimeout)
	}
	if SOCKSKeepAlive <= 0 {
		t.Errorf("SOCKSKeepAlive = %v, tem que ser positivo", SOCKSKeepAlive)
	}
}
