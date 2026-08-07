// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package events

import (
	"time"

	"wa-api/internal/wa-noise/types"
)

type NewsletterJoin struct {
	types.NewsletterMetadata
}

type NewsletterLeave struct {
	ID   types.JID            `json:"id"`
	Role types.NewsletterRole `json:"role"`
}

type NewsletterMuteChange struct {
	ID   types.JID                 `json:"id"`
	Mute types.NewsletterMuteState `json:"mute"`
}

type NewsletterLiveUpdate struct {
	JID      types.JID
	Time     time.Time
	Messages []*types.NewsletterMessage
}
