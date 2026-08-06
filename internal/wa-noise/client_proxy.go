// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package whatsmeow implements a client for interacting with the WhatsApp web multidevice API.
package whatsmeow

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// SetProxyAddress is a helper method that parses a URL string and calls SetProxy or SetSOCKSProxy based on the URL scheme.
//
// Returns an error if url.Parse fails to parse the given address.
func (cli *Client) SetProxyAddress(addr string, opts ...SetProxyOptions) error {
	if addr == "" {
		cli.SetProxy(nil, opts...)
		return nil
	}
	parsed, err := url.Parse(addr)
	if err != nil {
		return err
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		cli.SetProxy(http.ProxyURL(parsed), opts...)
	} else if parsed.Scheme == "socks5" {
		px, err := proxy.FromURL(parsed, &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		})
		if err != nil {
			return err
		}
		cli.SetSOCKSProxy(px, opts...)
	} else {
		return fmt.Errorf("unsupported proxy scheme %q", parsed.Scheme)
	}
	return nil
}

type Proxy = func(*http.Request) (*url.URL, error)

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
func (cli *Client) SetProxy(proxy Proxy, opts ...SetProxyOptions) {
	var opt SetProxyOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	transport := (http.DefaultTransport.(*http.Transport)).Clone()
	transport.Proxy = proxy
	cli.setTransport(transport, opt)
}

type SetProxyOptions struct {
	// If NoWebsocket is true, the proxy won't be used for the websocket
	NoWebsocket bool
	// If OnlyLogin is true, the proxy will be used for the pre-login websocket, but not the post-login one
	OnlyLogin bool
	// If NoMedia is true, the proxy won't be used for media uploads/downloads
	NoMedia bool
}

// SetSOCKSProxy sets a SOCKS5 proxy to use for WhatsApp web websocket connections and media uploads/downloads.
//
// Same details as SetProxy apply, but using a different proxy for the websocket and media is not currently supported.
func (cli *Client) SetSOCKSProxy(px proxy.Dialer, opts ...SetProxyOptions) {
	var opt SetProxyOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	transport := (http.DefaultTransport.(*http.Transport)).Clone()
	pxc := px.(proxy.ContextDialer)
	transport.DialContext = pxc.DialContext
	cli.setTransport(transport, opt)
}

func (cli *Client) setTransport(transport *http.Transport, opt SetProxyOptions) {
	if !opt.NoWebsocket {
		cli.preLoginHTTP.Transport = transport
		if !opt.OnlyLogin {
			cli.websocketHTTP.Transport = transport
		}
	}
	if !opt.NoMedia {
		cli.mediaHTTP.Transport = transport
	}
}

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
