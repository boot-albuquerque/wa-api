// Package proxyconf monta os http.Transport usados pelo cliente a partir de uma
// configuracao de proxy, e decide em quais dos tres http.Client o transport
// entra.
//
// O pacote nao guarda estado e nao conhece o *Client: recebe os tres ponteiros
// de http.Client em Clients e escreve o campo Transport de cada um. Substituir
// os proprios ponteiros (SetMediaHTTPClient e companhia) continua sendo da
// raiz, que e' quem tem os campos. Ver PATCHES.md, "Fase F/G — lote 10".
package proxyconf

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// Parametros do dialer usado quando um proxy SOCKS5 e' montado a partir de uma
// URL por TransportForAddress.
const (
	SOCKSDialTimeout = 30 * time.Second
	SOCKSKeepAlive   = 30 * time.Second
)

// Proxy e' a assinatura de http.Transport.Proxy. A raiz reexporta este tipo
// como whatsmeow.Proxy.
type Proxy = func(*http.Request) (*url.URL, error)

// Options diz em quais dos tres http.Client o transport entra. A raiz reexporta
// este tipo como whatsmeow.SetProxyOptions — e' API que consumidores em
// pkg/infra usam pelo nome antigo, entao o alias la' e' obrigatorio.
type Options struct {
	// If NoWebsocket is true, the proxy won't be used for the websocket
	NoWebsocket bool
	// If OnlyLogin is true, the proxy will be used for the pre-login websocket, but not the post-login one
	OnlyLogin bool
	// If NoMedia is true, the proxy won't be used for media uploads/downloads
	NoMedia bool
}

// Clients sao os tres http.Client do cliente. Os ponteiros sao usados como
// destino da escrita: Apply mexe no campo Transport de cada um, nunca troca o
// ponteiro.
type Clients struct {
	PreLogin  *http.Client
	Websocket *http.Client
	Media     *http.Client
}

// baseTransport e' um clone do transport padrao do net/http, ponto de partida
// de todos os transports montados aqui. Clonar (em vez de reaproveitar
// http.DefaultTransport) evita que a configuracao de proxy de uma sessao vaze
// para o resto do processo.
func baseTransport() *http.Transport {
	return (http.DefaultTransport.(*http.Transport)).Clone()
}

// TransportForProxy monta o transport de um proxy HTTP. p nil desliga a leitura
// de proxy das variaveis de ambiente, que e' o que a documentacao de
// Client.SetProxy promete.
func TransportForProxy(p Proxy) *http.Transport {
	t := baseTransport()
	t.Proxy = p
	return t
}

// TransportForDialer monta o transport de um proxy SOCKS5 a partir de um
// proxy.Dialer qualquer.
func TransportForDialer(px proxy.Dialer) *http.Transport {
	t := baseTransport()
	t.DialContext = ContextDialerFor(px)
	return t
}

// TransportForAddress monta o transport correspondente a uma URL de proxy.
//
// addr vazio devolve o transport sem proxy — e' assim que se desliga a leitura
// de proxy do ambiente, e era o que SetProxyAddress("") ja' fazia chamando
// SetProxy(nil).
func TransportForAddress(addr string) (*http.Transport, error) {
	if addr == "" {
		return TransportForProxy(nil), nil
	}
	parsed, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	switch parsed.Scheme {
	case "http", "https":
		return TransportForProxy(http.ProxyURL(parsed)), nil
	case "socks5":
		px, err := proxy.FromURL(parsed, &net.Dialer{
			Timeout:   SOCKSDialTimeout,
			KeepAlive: SOCKSKeepAlive,
		})
		if err != nil {
			return nil, err
		}
		return TransportForDialer(px), nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", parsed.Scheme)
	}
}

// ContextDialerFor adapta um proxy.Dialer qualquer para a assinatura de
// http.Transport.DialContext.
//
// A type assertion crua que estava aqui (`px.(proxy.ContextDialer)`) entrava em
// panic para qualquer Dialer que nao implementasse ContextDialer — e
// SetSOCKSProxy e' API *exportada*, entao o Dialer vem do chamador. O caminho
// interno (proxy.FromURL em TransportForAddress) devolve um dialer que
// implementa, mas isso nao vale para um dialer customizado.
//
// O fallback disca sem contexto de proposito. A alternativa — nao instalar o
// proxy — faria o trafego sair direto, sem proxy e sem aviso, o que e' pior que
// perder o cancelamento: um proxy pedido e silenciosamente ignorado vaza o
// endereco real do cliente.
func ContextDialerFor(px proxy.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if pxc, ok := px.(proxy.ContextDialer); ok {
		return pxc.DialContext
	}
	return func(_ context.Context, network, addr string) (net.Conn, error) {
		return px.Dial(network, addr)
	}
}

// Apply instala transport nos http.Client que opt permitir.
//
// Nao ha' guarda de nil, de proposito: e' literal ao setTransport de onde esta
// funcao saiu. Os tres http.Client sao sempre preenchidos por NewClient, mas
// Client.SetMediaHTTPClient e companhia aceitam nil do chamador, e nesse caso
// uma chamada de proxy posterior entra em panic. E' um defeito pre-existente,
// fora do escopo do lote 10; registrado em HOUSEKEEP.md.
func Apply(c Clients, transport *http.Transport, opt Options) {
	if !opt.NoWebsocket {
		c.PreLogin.Transport = transport
		if !opt.OnlyLogin {
			c.Websocket.Transport = transport
		}
	}
	if !opt.NoMedia {
		c.Media.Transport = transport
	}
}
