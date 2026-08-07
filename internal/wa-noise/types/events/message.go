// Copyright (c) 2021 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package events

import (
	"time"

	armadillo "wa-api/internal/wa-noise/protocol/proto"
	"wa-api/internal/wa-noise/protocol/proto/instamadilloTransportPayload"
	"wa-api/internal/wa-noise/protocol/proto/waArmadilloApplication"
	"wa-api/internal/wa-noise/protocol/proto/waConsumerApplication"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/proto/waHistorySync"
	"wa-api/internal/wa-noise/protocol/proto/waMsgApplication"
	"wa-api/internal/wa-noise/protocol/proto/waMsgTransport"
	"wa-api/internal/wa-noise/protocol/proto/waWeb"
	"wa-api/internal/wa-noise/types"
)

// HistorySync is emitted when the phone has sent a blob of historical messages.
type HistorySync struct {
	Data *waHistorySync.HistorySync
}

type DecryptFailMode string

const (
	DecryptFailShow DecryptFailMode = ""
	DecryptFailHide DecryptFailMode = "hide"
)

type UnavailableType string

const (
	UnavailableTypeUnknown  UnavailableType = ""
	UnavailableTypeViewOnce UnavailableType = "view_once"
)

// UndecryptableMessage is emitted when receiving a new message that failed to decrypt.
//
// The library will automatically ask the sender to retry. If the sender resends the message,
// and it's decryptable, then it will be emitted as a normal Message event.
//
// The UndecryptableMessage event may also be repeated if the resent message is also undecryptable.
type UndecryptableMessage struct {
	Info types.MessageInfo

	// IsUnavailable is true if the recipient device didn't send a ciphertext to this device at all
	// (as opposed to sending a ciphertext, but the ciphertext not being decryptable).
	IsUnavailable bool
	// Some message types are intentionally unavailable. Such types usually have a type specified here.
	UnavailableType UnavailableType

	DecryptFailMode DecryptFailMode
}

type NewsletterMessageMeta struct {
	// When a newsletter message is edited, the message isn't wrapped in an EditedMessage like normal messages.
	// Instead, the message is the new content, the ID is the original message ID, and the edit timestamp is here.
	EditTS time.Time
	// This is the timestamp of the original message for edits.
	OriginalTS time.Time
}

// Message is emitted when receiving a new message.
type Message struct {
	Info    types.MessageInfo // Information about the message like the chat and sender IDs
	Message *waE2E.Message    // The actual message struct

	IsEphemeral           bool // True if the message was unwrapped from an EphemeralMessage
	IsViewOnce            bool // True if the message was unwrapped from a ViewOnceMessage, ViewOnceMessageV2 or ViewOnceMessageV2Extension
	IsViewOnceV2          bool // True if the message was unwrapped from a ViewOnceMessageV2 or ViewOnceMessageV2Extension
	IsViewOnceV2Extension bool // True if the message was unwrapped from a ViewOnceMessageV2Extension
	IsDocumentWithCaption bool // True if the message was unwrapped from a DocumentWithCaptionMessage
	IsLottieSticker       bool // True if the message was unwrapped from a LottieStickerMessage
	IsBotInvoke           bool // True if the message was unwrapped from a BotInvokeMessage
	IsEdit                bool // True if the message was unwrapped from an EditedMessage

	// If this event was parsed from a WebMessageInfo (i.e. from a history sync or unavailable message request), the source data is here.
	SourceWebMsg *waWeb.WebMessageInfo
	// If this event is a response to an unavailable message request, the request ID is here.
	UnavailableRequestID types.MessageID
	// If the message was re-requested from the sender, this is the number of retries it took.
	RetryCount int

	NewsletterMeta *NewsletterMessageMeta

	// The raw message struct. This is the raw unmodified data, which means the actual message might
	// be wrapped in DeviceSentMessage, EphemeralMessage or ViewOnceMessage.
	RawMessage *waE2E.Message
}

type FBMessage struct {
	Info    types.MessageInfo               // Information about the message like the chat and sender IDs
	Message armadillo.MessageApplicationSub // The actual message struct

	// If the message was re-requested from the sender, this is the number of retries it took.
	RetryCount int

	Transport *waMsgTransport.MessageTransport // The first level of wrapping the message was in

	FBApplication *waMsgApplication.MessageApplication           // The second level of wrapping the message was in, for FB messages
	IGTransport   *instamadilloTransportPayload.TransportPayload // The second level of wrapping the message was in, for IG messages
}

func (evt *FBMessage) GetConsumerApplication() *waConsumerApplication.ConsumerApplication {
	if consumerApp, ok := evt.Message.(*waConsumerApplication.ConsumerApplication); ok {
		return consumerApp
	}
	return nil
}

func (evt *FBMessage) GetArmadillo() *waArmadilloApplication.Armadillo {
	if armadillo, ok := evt.Message.(*waArmadilloApplication.Armadillo); ok {
		return armadillo
	}
	return nil
}

// UnwrapRaw fills the Message, IsEphemeral and IsViewOnce fields based on the raw message in the RawMessage field.
func (evt *Message) UnwrapRaw() *Message {
	evt.Message = evt.RawMessage
	if evt.Message.GetDeviceSentMessage().GetMessage() != nil {
		evt.Info.DeviceSentMeta = &types.DeviceSentMeta{
			DestinationJID: evt.Message.GetDeviceSentMessage().GetDestinationJID(),
			Phash:          evt.Message.GetDeviceSentMessage().GetPhash(),
		}
		evt.Message = evt.Message.GetDeviceSentMessage().GetMessage()
	}
	if evt.Message.GetBotInvokeMessage().GetMessage() != nil {
		evt.Message = evt.Message.GetBotInvokeMessage().GetMessage()
		evt.IsBotInvoke = true
	}
	if evt.Message.GetEphemeralMessage().GetMessage() != nil {
		evt.Message = evt.Message.GetEphemeralMessage().GetMessage()
		evt.IsEphemeral = true
	}
	if evt.Message.GetViewOnceMessage().GetMessage() != nil {
		evt.Message = evt.Message.GetViewOnceMessage().GetMessage()
		evt.IsViewOnce = true
	}
	if evt.Message.GetViewOnceMessageV2().GetMessage() != nil {
		evt.Message = evt.Message.GetViewOnceMessageV2().GetMessage()
		evt.IsViewOnce = true
		evt.IsViewOnceV2 = true
	}
	if evt.Message.GetViewOnceMessageV2Extension().GetMessage() != nil {
		evt.Message = evt.Message.GetViewOnceMessageV2Extension().GetMessage()
		evt.IsViewOnce = true
		evt.IsViewOnceV2 = true
		evt.IsViewOnceV2Extension = true
	}
	if evt.Message.GetLottieStickerMessage().GetMessage() != nil {
		evt.Message = evt.Message.GetLottieStickerMessage().GetMessage()
		evt.IsLottieSticker = true
	}
	if evt.Message.GetDocumentWithCaptionMessage().GetMessage() != nil {
		evt.Message = evt.Message.GetDocumentWithCaptionMessage().GetMessage()
		evt.IsDocumentWithCaption = true
	}
	if evt.Message.GetEditedMessage().GetMessage() != nil {
		evt.Message = evt.Message.GetEditedMessage().GetMessage()
		evt.IsEdit = true
	}
	if evt.Message != nil && evt.RawMessage != nil && evt.Message.MessageContextInfo == nil && evt.RawMessage.MessageContextInfo != nil {
		evt.Message.MessageContextInfo = evt.RawMessage.MessageContextInfo
	}
	return evt
}

// Deprecated: use types.ReceiptType directly
type ReceiptType = types.ReceiptType

// Deprecated: use types.ReceiptType* constants directly
const (
	ReceiptTypeDelivered = types.ReceiptTypeDelivered
	ReceiptTypeSender    = types.ReceiptTypeSender
	ReceiptTypeRetry     = types.ReceiptTypeRetry
	ReceiptTypeRead      = types.ReceiptTypeRead
	ReceiptTypeReadSelf  = types.ReceiptTypeReadSelf
	ReceiptTypePlayed    = types.ReceiptTypePlayed
)

// Receipt is emitted when an outgoing message is delivered to or read by another user, or when another device reads an incoming message.
//
// N.B. WhatsApp on Android sends message IDs from newest message to oldest, but WhatsApp on iOS sends them in the opposite order (oldest first).
type Receipt struct {
	types.MessageSource
	MessageIDs []types.MessageID
	Timestamp  time.Time
	Type       types.ReceiptType

	// When you read the message of another user in a group, this field contains the sender of the message.
	// For receipts from other users, the message sender is always you.
	MessageSender types.JID
}
