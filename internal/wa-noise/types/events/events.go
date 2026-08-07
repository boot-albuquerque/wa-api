// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package events contains all the events that whatsmeow.Client emits to functions registered with AddEventHandler.
package events

import (
	"fmt"
	"time"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/types"
)

// QR is emitted after connecting when there's no session data in the device store.
//
// The QR codes are available in the Codes slice. You should render the strings as QR codes one by
// one, switching to the next one whenever enough time has passed. WhatsApp web seems to show the
// first code for 60 seconds and all other codes for 20 seconds.
//
// When the QR code has been scanned and pairing is complete, PairSuccess will be emitted. If you
// run out of codes before scanning, the server will close the websocket, and you will have to
// reconnect to get more codes.
type QR struct {
	Codes []string
}

// PairSuccess is emitted after the QR code has been scanned with the phone and the handshake has
// been completed. Note that this is generally followed by a websocket reconnection, so you should
// wait for the Connected before trying to send anything.
type PairSuccess struct {
	ID           types.JID
	LID          types.JID
	BusinessName string
	Platform     string
}

// PairError is emitted when a pair-success event is received from the server, but finishing the pairing locally fails.
type PairError struct {
	ID           types.JID
	LID          types.JID
	BusinessName string
	Platform     string
	Error        error
}

// QRScannedWithoutMultidevice is emitted when the pairing QR code is scanned, but the phone didn't have multidevice enabled.
// The same QR code can still be scanned after this event, which means the user can just be told to enable multidevice and re-scan the code.
type QRScannedWithoutMultidevice struct{}

// Connected is emitted when the client has successfully connected to the WhatsApp servers
// and is authenticated. The user who the client is authenticated as will be in the device store
// at this point, which is why this event doesn't contain any data.
type Connected struct{}

// KeepAliveTimeout is emitted when the keepalive ping request to WhatsApp web servers times out.
//
// Currently, there's no automatic handling for these, but it's expected that the TCP connection will
// either start working again or notice it's dead on its own eventually. Clients may use this event to
// decide to force a disconnect+reconnect faster.
type KeepAliveTimeout struct {
	ErrorCount  int
	LastSuccess time.Time
}

// KeepAliveRestored is emitted if the keepalive pings start working again after some KeepAliveTimeout events.
// Note that if the websocket disconnects before the pings start working, this event will not be emitted.
type KeepAliveRestored struct{}

// PermanentDisconnect is a class of events emitted when the client will not auto-reconnect by default.
type PermanentDisconnect interface {
	PermanentDisconnectDescription() string
}

func (l *LoggedOut) PermanentDisconnectDescription() string     { return l.Reason.String() }
func (*StreamReplaced) PermanentDisconnectDescription() string  { return "stream replaced" }
func (*ClientOutdated) PermanentDisconnectDescription() string  { return "client outdated" }
func (*CATRefreshError) PermanentDisconnectDescription() string { return "CAT refresh failed" }
func (tb *TemporaryBan) PermanentDisconnectDescription() string {
	return fmt.Sprintf("temporarily banned: %s", tb.String())
}

func (cf *ConnectFailure) PermanentDisconnectDescription() string {
	return fmt.Sprintf("connect failure: %s", cf.Reason.String())
}

// LoggedOut is emitted when the client has been unpaired from the phone.
//
// This can happen while connected (stream:error messages) or right after connecting (connect failure messages).
//
// This will not be emitted when the logout is initiated by this client (using Client.LogOut()).
type LoggedOut struct {
	// OnConnect is true if the event was triggered by a connect failure message.
	// If it's false, the event was triggered by a stream:error message.
	OnConnect bool
	// If OnConnect is true, then this field contains the reason code.
	Reason ConnectFailureReason
}

// StreamReplaced is emitted when the client is disconnected by another client connecting with the same keys.
//
// This can happen if you accidentally start another process with the same session
// or otherwise try to connect twice with the same session.
type StreamReplaced struct{}

// ManualLoginReconnect is emitted after login if DisableLoginAutoReconnect is set.
type ManualLoginReconnect struct{}

// TemporaryBan is emitted when there's a connection failure with the ConnectFailureTempBanned reason code.
type TemporaryBan struct {
	Code   TempBanReason
	Expire time.Duration
}

func (tb *TemporaryBan) String() string {
	if tb.Expire == 0 {
		return fmt.Sprintf("You've been temporarily banned: %v", tb.Code)
	}
	return fmt.Sprintf("You've been temporarily banned: %v. The ban expires in %v", tb.Code, tb.Expire)
}

// ConnectFailure is emitted when the WhatsApp server sends a <failure> node with an unknown reason.
//
// Known reasons are handled internally and emitted as different events (e.g. LoggedOut and TemporaryBan).
type ConnectFailure struct {
	Reason  ConnectFailureReason
	Message string
	Raw     *waBinary.Node
}

// ClientOutdated is emitted when the WhatsApp server rejects the connection with the ConnectFailureClientOutdated code.
type ClientOutdated struct{}

type CATRefreshError struct {
	Error error
}

// StreamError is emitted when the WhatsApp server sends a <stream:error> node with an unknown code.
//
// Known codes are handled internally and emitted as different events (e.g. LoggedOut).
type StreamError struct {
	Code string
	Raw  *waBinary.Node
}

// Disconnected is emitted when the websocket is closed by the server.
type Disconnected struct{}

// OfflineSyncPreview is emitted right after connecting if the server is going to send events that the client missed during downtime.
type OfflineSyncPreview struct {
	Total int

	AppDataChanges int
	Messages       int
	Notifications  int
	Receipts       int
}

// OfflineSyncCompleted is emitted after the server has finished sending missed events.
type OfflineSyncCompleted struct {
	Count int
}
