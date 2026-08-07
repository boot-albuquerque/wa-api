// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

// Tags of the nodes that make up an outgoing <message> stanza, in both the
// waE2E path (send*.go) and the v3/FB path (sendfb*.go).
const (
	messageNodeTag        = "message"
	participantsNodeTag   = "participants"
	participantToNodeTag  = "to"
	encNodeTag            = "enc"
	plaintextNodeTag      = "plaintext"
	deviceIdentityNodeTag = "device-identity"
	bizNodeTag            = "biz"
	frankingNodeTag       = "franking"
	frankingTagNodeTag    = "franking_tag"
	traceNodeTag          = "trace"
	traceRequestIDNodeTag = "request_id"
	tcTokenNodeTag        = "tctoken"
	csTokenNodeTag        = "cstoken"
)

// Attributes of the outgoing <message> stanza.
const (
	msgAttrID               = "id"
	msgAttrType             = "type"
	msgAttrTo               = "to"
	msgAttrCategory         = "category"
	msgAttrEdit             = "edit"
	msgAttrPHash            = "phash"
	msgAttrMediaID          = "media_id"
	msgAttrAddressingMode   = "addressing_mode"
	msgAttrPushPriority     = "push_priority"
	msgAttrPrivacySensitive = "privacy_sensitive"
)

// Values of the <message> attributes above. The message type values mirror what
// msgattrs.GetTypeFromMessage returns; they are compared against, never
// produced, by this package.
const (
	msgTypeText     = "text"
	msgTypePoll     = "poll"
	msgTypeReaction = "reaction"

	msgCategoryPeer = "peer"

	// pushPriorityHigh asks the server to wake the recipient for app state
	// sync key requests; pushPriorityHighForce does the same for on-demand
	// history sync, which additionally is marked privacy sensitive.
	pushPriorityHigh      = "high"
	pushPriorityHighForce = "high_force"
	privacySensitiveOn    = "1"
)

// Attributes of the <enc> node carrying a Signal ciphertext, plus the single
// attribute of the <to> node that wraps it per recipient device.
const (
	encAttrVersion     = "v"
	encAttrType        = "type"
	encAttrMediaType   = "mediatype"
	encAttrDecryptFail = "decrypt-fail"

	participantToAttrJID = "jid"
)

// Values of the <enc> `type` attribute: a normal Signal message, a message that
// also establishes the session from a prekey bundle, and a group (sender key)
// message.
const (
	encTypeMsg       = "msg"
	encTypePreKeyMsg = "pkmsg"
	encTypeSenderKey = "skmsg"
)

// Values of the <enc> `v` attribute. The waE2E path is version 2; the v3/FB
// group path writes "3" as a string here (the per-device v3 nodes use the
// numeric FBMessageVersion instead — see sendfb_encrypt.go).
const (
	encVersionSignal = "2"
	encVersionFB     = "3"
)

// Attributes and values of the <meta> node inside an outgoing message stanza.
// This is a different node from the <meta> built out of SendRequestExtra.Meta
// (see send_prepare.go), even though both use metaNodeTag.
const (
	metaAttrAppData     = "appdata"
	metaAttrPollType    = "polltype"
	metaAttrDecryptFail = "decrypt-fail"

	metaAppDataDefault = "default"
	pollTypeCreation   = "creation"
	pollTypeVote       = "vote"
)

// participantListHashPrefix is the version prefix of the participant list hash
// sent as the `phash` attribute, and participantListHashLength the number of
// bytes of the SHA-256 digest that go into it.
const (
	participantListHashPrefix = "2"
	participantListHashLength = 6
)

// frankingKeySize is the length in bytes of the random HMAC key generated per
// v3/FB message to produce its franking tag.
const frankingKeySize = 32

// frankingVersion is the franking scheme version advertised in the v3/FB
// message application metadata.
const frankingVersion = 0
