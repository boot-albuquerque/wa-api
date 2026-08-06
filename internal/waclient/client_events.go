// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package whatsmeow implements a client for interacting with the WhatsApp web multidevice API.
package whatsmeow

import (
	"context"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	waBinary "wa-api/internal/waclient/binary"
)

// EventHandler is a function that can handle events from WhatsApp.
type EventHandler func(evt any)
type EventHandlerWithSuccessStatus func(evt any) bool
type nodeHandler func(ctx context.Context, node *waBinary.Node)

var nextHandlerID uint32

type wrappedEventHandler struct {
	fn EventHandlerWithSuccessStatus
	id uint32
}

// Size of buffer for the channel that all incoming XML nodes go through.
// In general it shouldn't go past a few buffered messages, but the channel is big to be safe.
const handlerQueueSize = 2048

// AddEventHandler registers a new function to receive all events emitted by this client.
//
// The returned integer is the event handler ID, which can be passed to RemoveEventHandler to remove it.
//
// All registered event handlers will receive all events. You should use a type switch statement to
// filter the events you want:
//
//	func myEventHandler(evt interface{}) {
//		switch v := evt.(type) {
//		case *events.Message:
//			fmt.Println("Received a message!")
//		case *events.Receipt:
//			fmt.Println("Received a receipt!")
//		}
//	}
//
// If you want to access the Client instance inside the event handler, the recommended way is to
// wrap the whole handler in another struct:
//
//	type MyClient struct {
//		WAClient *whatsmeow.Client
//		eventHandlerID uint32
//	}
//
//	func (mycli *MyClient) register() {
//		mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.myEventHandler)
//	}
//
//	func (mycli *MyClient) myEventHandler(evt interface{}) {
//		// Handle event and access mycli.WAClient
//	}
func (cli *Client) AddEventHandler(handler EventHandler) uint32 {
	return cli.AddEventHandlerWithSuccessStatus(func(evt any) bool {
		handler(evt)
		return true
	})
}

func (cli *Client) AddEventHandlerWithSuccessStatus(handler EventHandlerWithSuccessStatus) uint32 {
	nextID := atomic.AddUint32(&nextHandlerID, 1)
	cli.eventHandlersLock.Lock()
	cli.eventHandlers = append(cli.eventHandlers, wrappedEventHandler{handler, nextID})
	cli.eventHandlersLock.Unlock()
	return nextID
}

// RemoveEventHandler removes a previously registered event handler function.
// If the function with the given ID is found, this returns true.
//
// N.B. Do not run this directly from an event handler. That would cause a deadlock because the
// event dispatcher holds a read lock on the event handler list, and this method wants a write lock
// on the same list. Instead run it in a goroutine:
//
//	func (mycli *MyClient) myEventHandler(evt interface{}) {
//		if noLongerWantEvents {
//			go mycli.WAClient.RemoveEventHandler(mycli.eventHandlerID)
//		}
//	}
func (cli *Client) RemoveEventHandler(id uint32) bool {
	cli.eventHandlersLock.Lock()
	defer cli.eventHandlersLock.Unlock()
	for index := range cli.eventHandlers {
		if cli.eventHandlers[index].id == id {
			if index == 0 {
				cli.eventHandlers[0].fn = nil
				cli.eventHandlers = cli.eventHandlers[1:]
				return true
			} else if index < len(cli.eventHandlers)-1 {
				copy(cli.eventHandlers[index:], cli.eventHandlers[index+1:])
			}
			cli.eventHandlers[len(cli.eventHandlers)-1].fn = nil
			cli.eventHandlers = cli.eventHandlers[:len(cli.eventHandlers)-1]
			return true
		}
	}
	return false
}

// RemoveEventHandlers removes all event handlers that have been registered with AddEventHandler
func (cli *Client) RemoveEventHandlers() {
	cli.eventHandlersLock.Lock()
	cli.eventHandlers = make([]wrappedEventHandler, 0, 1)
	cli.eventHandlersLock.Unlock()
}

func (cli *Client) handleFrame(ctx context.Context, data []byte) {
	decompressed, err := waBinary.Unpack(data)
	if err != nil {
		cli.Log.Warnf("Failed to decompress frame: %v", err)
		cli.Log.Debugf("Errored frame hex: %s", hex.EncodeToString(data))
		return
	}
	node, err := waBinary.Unmarshal(decompressed)
	if err != nil {
		cli.Log.Warnf("Failed to decode node in frame: %v", err)
		cli.Log.Debugf("Errored frame hex: %s", hex.EncodeToString(decompressed))
		return
	}
	cli.recvLog.Debugf("%s", node.XMLString())
	if node.Tag == "xmlstreamend" {
		if !cli.isExpectedDisconnect() {
			cli.Log.Warnf("Received stream end frame")
		}
		// TODO should we do something else?
	} else if cli.receiveResponse(ctx, node) {
		// handled
	} else if _, ok := cli.nodeHandlers[node.Tag]; ok {
		select {
		case cli.handlerQueue <- node:
		case <-ctx.Done():
		default:
			cli.Log.Warnf("Handler queue is full, message ordering is no longer guaranteed")
			go func() {
				select {
				case cli.handlerQueue <- node:
				case <-ctx.Done():
				}
			}()
		}
	} else if node.Tag != "ack" {
		cli.Log.Debugf("Didn't handle WhatsApp node %s", node.Tag)
	}
}

func (cli *Client) handlerQueueLoop(evtCtx, connCtx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	ticker.Stop()
	cli.Log.Debugf("Starting handler queue loop")
Loop:
	for {
		select {
		case node := <-cli.handlerQueue:
			doneChan := make(chan struct{}, 1)
			start := time.Now()
			go func() {
				cli.nodeHandlers[node.Tag](evtCtx, node)
				duration := time.Since(start)
				doneChan <- struct{}{}
				if duration > 5*time.Second {
					cli.Log.Warnf("Node handling took %s for %s", duration, node.XMLString())
				}
			}()
			ticker.Reset(30 * time.Second)
			for i := 0; i < 10; i++ {
				select {
				case <-doneChan:
					ticker.Stop()
					continue Loop
				case <-ticker.C:
					cli.Log.Warnf("Node handling is taking long for %s (started %s ago)", node.XMLString(), time.Since(start))
				}
			}
			cli.Log.Warnf("Continuing handling of %s in background as it's taking too long", node.XMLString())
			ticker.Stop()
		case <-connCtx.Done():
			cli.Log.Debugf("Closing handler queue loop")
			return
		}
	}
}

func (cli *Client) sendNodeAndGetData(ctx context.Context, node waBinary.Node) ([]byte, error) {
	if cli == nil {
		return nil, ErrClientIsNil
	}
	cli.socketLock.RLock()
	sock := cli.socket
	cli.socketLock.RUnlock()
	if sock == nil {
		return nil, ErrNotConnected
	}

	payload, err := waBinary.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal node: %w", err)
	}

	cli.sendLog.Debugf("%s", node.XMLString())
	return payload, sock.SendFrame(ctx, payload)
}

func (cli *Client) sendNode(ctx context.Context, node waBinary.Node) error {
	_, err := cli.sendNodeAndGetData(ctx, node)
	return err
}

func (cli *Client) dispatchEvent(evt any) (handlerFailed bool) {
	cli.eventHandlersLock.RLock()
	defer func() {
		cli.eventHandlersLock.RUnlock()
		err := recover()
		if err != nil {
			cli.Log.Errorf("Event handler panicked while handling a %T: %v\n%s", evt, err, debug.Stack())
		}
	}()
	for _, handler := range cli.eventHandlers {
		if !handler.fn(evt) {
			return true
		}
	}
	return false
}
