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

// ChatPresence is emitted when a chat state update (also known as typing notification) is received.
//
// Note that WhatsApp won't send you these updates unless you mark yourself as online:
//
//	client.SendPresence(types.PresenceAvailable)
type ChatPresence struct {
	types.MessageSource
	State types.ChatPresence      // The current state, either composing or paused
	Media types.ChatPresenceMedia // When composing, the type of message
}

// Presence is emitted when a presence update is received.
//
// Note that WhatsApp only sends you presence updates for individual users after you subscribe to them:
//
//	client.SubscribePresence(user JID)
type Presence struct {
	// The user whose presence event this is
	From types.JID
	// True if the user is now offline
	Unavailable bool
	// The time when the user was last online. This may be the zero value if the user has hid their last seen time.
	LastSeen time.Time
}

// Picture is emitted when a user's profile picture or group's photo is changed.
//
// You can use Client.GetProfilePictureInfo to get the actual image URL after this event.
type Picture struct {
	JID       types.JID // The user or group ID where the picture was changed.
	Author    types.JID // The user who changed the picture.
	Timestamp time.Time // The timestamp when the picture was changed.
	Remove    bool      // True if the picture was removed.
	PictureID string    // The new picture ID if it was not removed.
}

// UserAbout is emitted when a user's about status is changed.
type UserAbout struct {
	JID       types.JID // The user whose status was changed
	Status    string    // The new status
	Timestamp time.Time // The timestamp when the status was changed.
}

// IdentityChange is emitted when another user changes their primary device.
type IdentityChange struct {
	JID       types.JID
	Timestamp time.Time

	// Implicit will be set to true if the event was triggered by an untrusted identity error,
	// rather than an identity change notification from the server.
	Implicit bool
}

// PrivacySettings is emitted when the user changes their privacy settings.
type PrivacySettings struct {
	NewSettings         types.PrivacySettings
	GroupAddChanged     bool
	LastSeenChanged     bool
	StatusChanged       bool
	ProfileChanged      bool
	ReadReceiptsChanged bool
	OnlineChanged       bool
	CallAddChanged      bool
	MessagesChanged     bool
	DefenseChanged      bool
	StickersChanged     bool
}

type MediaRetryError struct {
	Code int
}

// MediaRetry is emitted when the phone sends a response to a media retry request.
type MediaRetry struct {
	Ciphertext []byte
	IV         []byte

	// Sometimes there's an unencrypted media retry error. In these cases, Ciphertext and IV will be nil.
	Error *MediaRetryError

	Timestamp time.Time // The time of the response.

	MessageID types.MessageID // The ID of the message.
	ChatID    types.JID       // The chat ID where the message was sent.
	SenderID  types.JID       // The user who sent the message. Only present in groups.
	FromMe    bool            // Whether the message was sent by the current user or someone else.
}

type BlocklistAction string

const (
	BlocklistActionDefault BlocklistAction = ""
	BlocklistActionModify  BlocklistAction = "modify"
)

// Blocklist is emitted when the user's blocked user list is changed.
type Blocklist struct {
	// Action specifies what happened. If it's empty, there should be a list of changes in the Changes list.
	// If it's "modify", then the Changes list will be empty and the whole blocklist should be re-requested.
	Action    BlocklistAction
	DHash     string
	PrevDHash string
	Changes   []BlocklistChange
}

type BlocklistChangeAction string

const (
	BlocklistChangeActionBlock   BlocklistChangeAction = "block"
	BlocklistChangeActionUnblock BlocklistChangeAction = "unblock"
)

type BlocklistChange struct {
	JID    types.JID
	Action BlocklistChangeAction
}
