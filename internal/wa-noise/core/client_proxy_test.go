// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
)

// proxyTestClient traz so' os tres http.Client que setTransport mexe.
func proxyTestClient() *Client {
	return &Client{
		mediaHTTP:     &http.Client{},
		websocketHTTP: &http.Client{},
		preLoginHTTP:  &http.Client{},
	}
}

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

// BUG DO LOTE 10: SetSOCKSProxy entrava em panic com um Dialer sem DialContext.
//
// SetSOCKSProxy e' API exportada e aceita qualquer proxy.Dialer. A assertion
// `px.(proxy.ContextDialer)` derrubava o processo para qualquer dialer
// customizado — e proxy.Dialer, a interface do parametro, nao exige
// DialContext.
func TestSetSOCKSProxyWithNonContextDialerDoesNotPanic(t *testing.T) {
	var called int
	cli := proxyTestClient()

	cli.SetSOCKSProxy(plainDialer{called: &called})

	transport, ok := cli.websocketHTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, queria *http.Transport", cli.websocketHTTP.Transport)
	}
	if transport.DialContext == nil {
		t.Fatal("DialContext nao foi instalado; o trafego sairia sem proxy")
	}
	// O proxy tem que ser realmente usado: instalar um DialContext que ignora o
	// dialer vazaria o endereco real do cliente.
	_, _ = transport.DialContext(t.Context(), "tcp", "example.com:443")
	if called != 1 {
		t.Errorf("Dial do proxy chamado %d vezes, queria 1", called)
	}
}

// Quando o dialer implementa ContextDialer, e' o DialContext dele que tem que
// ser usado — o adaptador nao pode descartar o contexto a toa.
func TestSetSOCKSProxyPrefersContextDialer(t *testing.T) {
	var dialCalled, ctxDialCalled int
	cli := proxyTestClient()

	cli.SetSOCKSProxy(ctxDialer{dialCalled: &dialCalled, ctxDialCalled: &ctxDialCalled})

	transport := cli.websocketHTTP.Transport.(*http.Transport)
	_, _ = transport.DialContext(t.Context(), "tcp", "example.com:443")

	if ctxDialCalled != 1 {
		t.Errorf("DialContext chamado %d vezes, queria 1", ctxDialCalled)
	}
	if dialCalled != 0 {
		t.Errorf("Dial sem contexto chamado %d vezes, queria 0", dialCalled)
	}
}

// --- SetProxyAddress: roteamento por esquema ---

func TestSetProxyAddressSchemes(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		wantErr string
	}{
		{name: "http", addr: "http://proxy.local:8080"},
		{name: "https", addr: "https://proxy.local:8443"},
		{name: "socks5", addr: "socks5://proxy.local:1080"},
		{name: "esquema desconhecido", addr: "ftp://proxy.local", wantErr: "unsupported proxy scheme"},
		{name: "url invalida", addr: "://sem-esquema", wantErr: "missing protocol scheme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli := proxyTestClient()
			err := cli.SetProxyAddress(tc.addr)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, queria conter %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, queria nil", err)
			}
			if cli.websocketHTTP.Transport == nil {
				t.Error("o transport do websocket nao foi configurado")
			}
		})
	}
}

// Endereco vazio zera o proxy em vez de virar erro de parse — e' como se
// desliga a leitura de https_proxy do ambiente.
func TestSetProxyAddressEmptyClearsProxy(t *testing.T) {
	cli := proxyTestClient()
	if err := cli.SetProxyAddress(""); err != nil {
		t.Fatalf("err = %v, queria nil", err)
	}
	transport, ok := cli.websocketHTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, queria *http.Transport", cli.websocketHTTP.Transport)
	}
	if transport.Proxy != nil {
		t.Error("endereco vazio tem que deixar Proxy nil")
	}
}

// --- SetProxyOptions ---

// Cada flag desliga exatamente um destino. Errar aqui manda trafego pelo canal
// errado — por exemplo o websocket por um proxy que so' deveria servir midia.
func TestSetProxyOptionsSelectTargets(t *testing.T) {
	cases := []struct {
		name                            string
		opt                             SetProxyOptions
		wantPreLogin, wantWS, wantMedia bool
	}{
		{name: "padrao", opt: SetProxyOptions{}, wantPreLogin: true, wantWS: true, wantMedia: true},
		{name: "NoWebsocket", opt: SetProxyOptions{NoWebsocket: true}, wantMedia: true},
		{name: "OnlyLogin", opt: SetProxyOptions{OnlyLogin: true}, wantPreLogin: true, wantMedia: true},
		{name: "NoMedia", opt: SetProxyOptions{NoMedia: true}, wantPreLogin: true, wantWS: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cli := proxyTestClient()
			cli.SetProxy(http.ProxyURL(nil), tc.opt)

			if got := cli.preLoginHTTP.Transport != nil; got != tc.wantPreLogin {
				t.Errorf("preLoginHTTP configurado = %v, queria %v", got, tc.wantPreLogin)
			}
			if got := cli.websocketHTTP.Transport != nil; got != tc.wantWS {
				t.Errorf("websocketHTTP configurado = %v, queria %v", got, tc.wantWS)
			}
			if got := cli.mediaHTTP.Transport != nil; got != tc.wantMedia {
				t.Errorf("mediaHTTP configurado = %v, queria %v", got, tc.wantMedia)
			}
		})
	}
}

// Os setters de http.Client trocam o cliente inteiro, um destino de cada vez.
func TestSetHTTPClientsAreIndependent(t *testing.T) {
	cli := proxyTestClient()
	media, ws, preLogin := &http.Client{}, &http.Client{}, &http.Client{}

	cli.SetMediaHTTPClient(media)
	cli.SetWebsocketHTTPClient(ws)
	cli.SetPreLoginHTTPClient(preLogin)

	if cli.mediaHTTP != media || cli.websocketHTTP != ws || cli.preLoginHTTP != preLogin {
		t.Error("cada setter tem que trocar so' o seu proprio cliente")
	}
}
