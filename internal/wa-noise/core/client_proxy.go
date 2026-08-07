// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package whatsmeow implements a client for interacting with the WhatsApp web multidevice API.
package whatsmeow

import (
	"net/http"

	"golang.org/x/net/proxy"

	proxyconf "wa-api/internal/wa-noise/runtime/proxy"
)

// Proxy e SetProxyOptions sao aliases para os tipos de proxyconf. Precisam ser
// **aliases**, e nao tipos novos: `whatsmeow.SetProxyOptions` aparece nas
// assinaturas de SetProxy/SetSOCKSProxy/SetProxyAddress e e' usado por
// consumidores fora do fork (pkg/infra/wa-noise/session). Um tipo distinto
// quebraria esses chamadores.
type (
	Proxy           = proxyconf.Proxy
	SetProxyOptions = proxyconf.Options
)

// firstOpt reproduz o `var opt X; if len(opts) > 0 { opt = opts[0] }` que os
// tres metodos repetiam: opcoes alem da primeira sao ignoradas, e nenhuma
// opcao vale o zero-value.
func firstOpt(opts []SetProxyOptions) SetProxyOptions {
	if len(opts) > 0 {
		return opts[0]
	}
	return SetProxyOptions{}
}

// SetProxyAddress is a helper method that parses a URL string and calls SetProxy or SetSOCKSProxy based on the URL scheme.
//
// Returns an error if url.Parse fails to parse the given address.
func (cli *Client) SetProxyAddress(addr string, opts ...SetProxyOptions) error {
	transport, err := proxyconf.TransportForAddress(addr)
	if err != nil {
		return err
	}
	cli.setTransport(transport, firstOpt(opts))
	return nil
}

// SetProxy sets a HTTP proxy to use for WhatsApp web websocket connections and media uploads/downloads.
//
// Must be called before Connect() to take effect in the websocket connection.
// If you want to change the proxy after connecting, you must call Disconnect() and then Connect() again manually.
//
// By default, the client will find the proxy from the https_proxy environment variable like Go's net/http does.
//
// To disable reading proxy info from environment variables, explicitly set the proxy to nil:
//
//	cli.SetProxy(nil)
//
// To use a different proxy for the websocket and media, pass a function that checks the request path or headers:
//
//	cli.SetProxy(func(r *http.Request) (*url.URL, error) {
//		if r.URL.Host == "web.whatsapp.com" && r.URL.Path == "/ws/chat" {
//			return websocketProxyURL, nil
//		} else {
//			return mediaProxyURL, nil
//		}
//	})
func (cli *Client) SetProxy(p Proxy, opts ...SetProxyOptions) {
	cli.setTransport(proxyconf.TransportForProxy(p), firstOpt(opts))
}

// SetSOCKSProxy sets a SOCKS5 proxy to use for WhatsApp web websocket connections and media uploads/downloads.
//
// Same details as SetProxy apply, but using a different proxy for the websocket and media is not currently supported.
func (cli *Client) SetSOCKSProxy(px proxy.Dialer, opts ...SetProxyOptions) {
	cli.setTransport(proxyconf.TransportForDialer(px), firstOpt(opts))
}

// setTransport instala transport nos http.Client que opt permitir. Continua
// existindo com este nome porque internals.go (gerado) o expoe como
// DangerousInternalClient.SetTransport.
func (cli *Client) setTransport(transport *http.Transport, opt SetProxyOptions) {
	proxyconf.Apply(proxyconf.Clients{
		PreLogin:  cli.preLoginHTTP,
		Websocket: cli.websocketHTTP,
		Media:     cli.mediaHTTP,
	}, transport, opt)
}

// Os tres setters abaixo **trocam o ponteiro** do http.Client, e por isso ficam
// na raiz: sao escrita direta em campo de Client, nao algo que proxyconf possa
// fazer a partir de Clients (que carrega copias dos ponteiros).

// SetMediaHTTPClient sets the HTTP client used to download media.
// This will overwrite any set proxy calls.
func (cli *Client) SetMediaHTTPClient(h *http.Client) {
	cli.mediaHTTP = h
}

// SetWebsocketHTTPClient sets the HTTP client used to establish the websocket connection for logged-in sessions.
// This will overwrite any set proxy calls.
func (cli *Client) SetWebsocketHTTPClient(h *http.Client) {
	cli.websocketHTTP = h
}

// SetPreLoginHTTPClient sets the HTTP client used to establish the websocket connection before login.
// This will overwrite any set proxy calls.
func (cli *Client) SetPreLoginHTTPClient(h *http.Client) {
	cli.preLoginHTTP = h
}
