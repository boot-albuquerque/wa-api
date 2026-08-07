// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package events

import (
	"fmt"
	"strconv"
)

// TempBanReason is an error code included in temp ban error events.
type TempBanReason int

const (
	TempBanSentToTooManyPeople    TempBanReason = 101
	TempBanBlockedByUsers         TempBanReason = 102
	TempBanCreatedTooManyGroups   TempBanReason = 103
	TempBanSentTooManySameMessage TempBanReason = 104
	TempBanBroadcastList          TempBanReason = 106
)

var tempBanReasonMessage = map[TempBanReason]string{
	TempBanSentToTooManyPeople:    "you sent too many messages to people who don't have you in their address books",
	TempBanBlockedByUsers:         "too many people blocked you",
	TempBanCreatedTooManyGroups:   "you created too many groups with people who don't have you in their address books",
	TempBanSentTooManySameMessage: "you sent the same message to too many people",
	TempBanBroadcastList:          "you sent too many messages to a broadcast list",
}

// String returns the reason code and a human-readable description of the ban reason.
func (tbr TempBanReason) String() string {
	msg, ok := tempBanReasonMessage[tbr]
	if !ok {
		msg = "you may have violated the terms of service (unknown error)"
	}
	return fmt.Sprintf("%d: %s", int(tbr), msg)
}

// ConnectFailureReason is an error code included in connection failure events.
type ConnectFailureReason int

const (
	ConnectFailureGeneric        ConnectFailureReason = 400
	ConnectFailureLoggedOut      ConnectFailureReason = 401
	ConnectFailureTempBanned     ConnectFailureReason = 402
	ConnectFailureMainDeviceGone ConnectFailureReason = 403 // this is now called LOCKED in the whatsapp web code
	ConnectFailureUnknownLogout  ConnectFailureReason = 406 // this is now called BANNED in the whatsapp web code

	ConnectFailureClientOutdated ConnectFailureReason = 405
	ConnectFailureBadUserAgent   ConnectFailureReason = 409

	ConnectFailureCATExpired ConnectFailureReason = 413
	ConnectFailureCATInvalid ConnectFailureReason = 414
	ConnectFailureNotFound   ConnectFailureReason = 415

	// Status code unknown (not in WA web)
	ConnectFailureClientUnknown ConnectFailureReason = 418

	ConnectFailureInternalServerError ConnectFailureReason = 500
	ConnectFailureExperimental        ConnectFailureReason = 501
	ConnectFailureServiceUnavailable  ConnectFailureReason = 503
)

var connectFailureReasonMessage = map[ConnectFailureReason]string{
	ConnectFailureLoggedOut:      "logged out from another device",
	ConnectFailureTempBanned:     "account temporarily banned",
	ConnectFailureMainDeviceGone: "primary device was logged out", // seems to happen for both bans and switching phones
	ConnectFailureUnknownLogout:  "logged out for unknown reason",
	ConnectFailureClientOutdated: "client is out of date",
	ConnectFailureBadUserAgent:   "client user agent was rejected",
	ConnectFailureCATExpired:     "messenger crypto auth token has expired",
	ConnectFailureCATInvalid:     "messenger crypto auth token is invalid",
}

// IsLoggedOut returns true if the client should delete session data due to this connect failure.
func (cfr ConnectFailureReason) IsLoggedOut() bool {
	return cfr == ConnectFailureLoggedOut || cfr == ConnectFailureMainDeviceGone || cfr == ConnectFailureUnknownLogout
}

func (cfr ConnectFailureReason) NumberString() string {
	return strconv.Itoa(int(cfr))
}

// String returns the reason code and a short human-readable description of the error.
func (cfr ConnectFailureReason) String() string {
	msg, ok := connectFailureReasonMessage[cfr]
	if !ok {
		msg = "unknown error"
	}
	return fmt.Sprintf("%d: %s", int(cfr), msg)
}
