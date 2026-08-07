// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package whatsmeow implements a client for interacting with the WhatsApp web multidevice API.
package whatsmeow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mau.fi/util/exhttp"

	"wa-api/internal/wa-noise/socket"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types/events"
	"wa-api/internal/wa-noise/util/keys"
)

func (cli *Client) getSocketWaitChan() <-chan struct{} {
	cli.socketLock.RLock()
	ch := cli.socketWait
	cli.socketLock.RUnlock()
	return ch
}

func (cli *Client) closeSocketWaitChan() {
	cli.socketLock.Lock()
	close(cli.socketWait)
	cli.socketWait = make(chan struct{})
	cli.socketLock.Unlock()
}

func (cli *Client) WaitForConnection(timeout time.Duration) bool {
	if cli == nil {
		return false
	}
	timeoutChan := time.After(timeout)
	cli.socketLock.RLock()
	for cli.socket == nil || !cli.socket.IsConnected() || !cli.IsLoggedIn() {
		ch := cli.socketWait
		cli.socketLock.RUnlock()
		select {
		case <-ch:
		case <-timeoutChan:
			return false
		case <-cli.expectedDisconnect.GetChan():
			return false
		}
		cli.socketLock.RLock()
	}
	cli.socketLock.RUnlock()
	return true
}

// Connect connects the client to the WhatsApp web websocket. After connection, it will either
// authenticate if there's data in the device store, or emit a QREvent to set up a new link.
func (cli *Client) Connect() error {
	return cli.ConnectContext(cli.BackgroundEventCtx)
}

func isRetryableConnectError(err error) bool {
	if exhttp.IsNetworkError(err) {
		return true
	}

	var statusErr socket.ErrWithStatusCode
	if errors.As(err, &statusErr) {
		_, retryable := retryableConnectStatusCodes[statusErr.StatusCode]
		return retryable
	}

	return errors.Is(err, socket.ErrDialFailed)
}

func (cli *Client) ConnectContext(ctx context.Context) error {
	if cli == nil {
		return ErrClientIsNil
	}

	cli.socketLock.Lock()
	defer cli.socketLock.Unlock()

	err := cli.unlockedConnect(ctx)
	if isRetryableConnectError(err) && cli.InitialAutoReconnect && cli.EnableAutoReconnect {
		cli.Log.Errorf("Initial connection failed but reconnecting in background (%v)", err)
		go cli.dispatchEvent(&events.Disconnected{})
		go cli.autoReconnect(ctx)
		return nil
	}
	return err
}

func (cli *Client) connect(ctx context.Context) error {
	cli.socketLock.Lock()
	defer cli.socketLock.Unlock()

	return cli.unlockedConnect(ctx)
}

func (cli *Client) unlockedConnect(ctx context.Context) error {
	if cli.Store.Deleted {
		return store.ErrDeviceDeleted
	}
	if cli.socket != nil {
		if !cli.socket.IsConnected() {
			cli.unlockedDisconnect()
		} else {
			return ErrAlreadyConnected
		}
	}

	cli.resetExpectedDisconnect()
	client := cli.websocketHTTP
	if cli.Store.ID == nil {
		client = cli.preLoginHTTP
	}
	fs := socket.NewFrameSocket(cli.Log.Sub("Socket"), client)
	if cli.MessengerConfig != nil {
		fs.URL = cli.MessengerConfig.WebsocketURL
		fs.HTTPHeaders.Set("Origin", cli.MessengerConfig.BaseURL)
		fs.HTTPHeaders.Set("User-Agent", cli.MessengerConfig.UserAgent)
		fs.HTTPHeaders.Set("Cache-Control", "no-cache")
		fs.HTTPHeaders.Set("Pragma", "no-cache")
		//fs.HTTPHeaders.Set("Sec-Fetch-Dest", "empty")
		//fs.HTTPHeaders.Set("Sec-Fetch-Mode", "websocket")
		//fs.HTTPHeaders.Set("Sec-Fetch-Site", "cross-site")
	}
	if err := fs.Connect(ctx); err != nil {
		fs.Close(0)
		return err
	} else if err = cli.doHandshake(ctx, fs, *keys.NewKeyPair()); err != nil {
		fs.Close(0)
		return fmt.Errorf("noise handshake failed: %w", err)
	}
	go cli.keepAliveLoop(ctx, fs.Context())
	go cli.handlerQueueLoop(ctx, fs.Context())
	return nil
}

// IsLoggedIn returns true after the client is successfully connected and authenticated on WhatsApp.
func (cli *Client) IsLoggedIn() bool {
	return cli != nil && cli.isLoggedIn.Load()
}

func (cli *Client) onDisconnect(ctx context.Context, ns *socket.NoiseSocket, remote bool) {
	ns.Stop(false, false)
	cli.socketLock.Lock()
	defer cli.socketLock.Unlock()
	if cli.socket == ns {
		cli.socket = nil
		cli.clearResponseWaiters(xmlStreamEndNode)
		if !cli.isExpectedDisconnect() && (cli.forceAutoReconnect.Swap(false) || remote) {
			cli.Log.Debugf("Emitting Disconnected event")
			go cli.dispatchEvent(&events.Disconnected{})
			go cli.autoReconnect(ctx)
		} else if remote {
			cli.Log.Debugf("OnDisconnect() called, but it was expected, so not emitting event")
		} else {
			cli.Log.Debugf("OnDisconnect() called after manual disconnection")
		}
	} else {
		cli.Log.Debugf("Ignoring OnDisconnect on different socket")
	}
}

func (cli *Client) expectDisconnect() {
	cli.forceAutoReconnect.Store(false)
	cli.expectedDisconnect.Set()
}

func (cli *Client) resetExpectedDisconnect() {
	cli.forceAutoReconnect.Store(false)
	cli.expectedDisconnect.Clear()
}

func (cli *Client) isExpectedDisconnect() bool {
	return cli.expectedDisconnect.IsSet()
}

func (cli *Client) autoReconnect(ctx context.Context) {
	if !cli.EnableAutoReconnect || cli.Store.ID == nil {
		return
	}
	for {
		autoReconnectDelay := time.Duration(cli.AutoReconnectErrors) * autoReconnectDelayStep
		cli.Log.Debugf("Automatically reconnecting after %v", autoReconnectDelay)
		cli.AutoReconnectErrors++
		if cli.expectedDisconnect.WaitTimeoutCtx(ctx, autoReconnectDelay) == nil {
			cli.Log.Debugf("Cancelling automatic reconnect due to expected disconnect")
			return
		} else if ctx.Err() != nil {
			cli.Log.Debugf("Cancelling automatic reconnect due to context cancellation")
			return
		}
		err := cli.connect(ctx)
		if errors.Is(err, ErrAlreadyConnected) {
			cli.Log.Debugf("Connect() said we're already connected after autoreconnect sleep")
			return
		} else if err != nil {
			if cli.expectedDisconnect.IsSet() {
				cli.Log.Debugf("Autoreconnect failed, but disconnect was expected, not reconnecting")
				return
			}
			cli.Log.Errorf("Error reconnecting after autoreconnect sleep: %v", err)
			if cli.AutoReconnectHook != nil && !cli.AutoReconnectHook(err) {
				cli.Log.Debugf("AutoReconnectHook returned false, not reconnecting")
				return
			}
		} else {
			return
		}
	}
}

// IsConnected checks if the client is connected to the WhatsApp web websocket.
// Note that this doesn't check if the client is authenticated. See the IsLoggedIn field for that.
func (cli *Client) IsConnected() bool {
	if cli == nil {
		return false
	}
	cli.socketLock.RLock()
	connected := cli.socket != nil && cli.socket.IsConnected()
	cli.socketLock.RUnlock()
	return connected
}

// Disconnect disconnects from the WhatsApp web websocket.
//
// This will not emit any events, the Disconnected event is only used when the
// connection is closed by the server or a network error.
func (cli *Client) Disconnect() {
	if cli == nil {
		return
	}
	cli.socketLock.Lock()
	cli.expectDisconnect()
	cli.unlockedDisconnect()
	cli.socketLock.Unlock()
	cli.clearDelayedMessageRequests()
}

// ResetConnection disconnects from the WhatsApp web websocket and forces an automatic reconnection.
// This will not do anything if the socket is already disconnected or if EnableAutoReconnect is false.
func (cli *Client) ResetConnection() {
	if cli == nil {
		return
	}
	cli.socketLock.Lock()
	cli.forceAutoReconnect.Store(true)
	if cli.socket != nil {
		cli.socket.Stop(true, true)
		cli.clearResponseWaiters(xmlStreamEndNode)
	}
	cli.socketLock.Unlock()
}

// Disconnect closes the websocket connection.
func (cli *Client) unlockedDisconnect() {
	if cli.socket != nil {
		cli.socket.Stop(true, false)
		cli.socket = nil
		cli.clearResponseWaiters(xmlStreamEndNode)
	}
}
