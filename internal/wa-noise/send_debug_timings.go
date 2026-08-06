// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"time"

	"github.com/rs/zerolog"
)

type MessageDebugTimings struct {
	LIDFetch time.Duration
	Queue    time.Duration

	Marshal         time.Duration
	GetParticipants time.Duration
	GetDevices      time.Duration
	GroupEncrypt    time.Duration
	PeerEncrypt     time.Duration

	Send  time.Duration
	Resp  time.Duration
	Retry time.Duration
}

func (mdt MessageDebugTimings) MarshalZerologObject(evt *zerolog.Event) {
	if mdt.LIDFetch != 0 {
		evt.Dur("lid_fetch", mdt.LIDFetch)
	}
	evt.Dur("queue", mdt.Queue)
	evt.Dur("marshal", mdt.Marshal)
	if mdt.GetParticipants != 0 {
		evt.Dur("get_participants", mdt.GetParticipants)
	}
	evt.Dur("get_devices", mdt.GetDevices)
	if mdt.GroupEncrypt != 0 {
		evt.Dur("group_encrypt", mdt.GroupEncrypt)
	}
	evt.Dur("peer_encrypt", mdt.PeerEncrypt)
	evt.Dur("send", mdt.Send)
	evt.Dur("resp", mdt.Resp)
	if mdt.Retry != 0 {
		evt.Dur("retry", mdt.Retry)
	}
}
