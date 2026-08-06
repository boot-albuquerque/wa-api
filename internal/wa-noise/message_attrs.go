// Copyright (c) 2022 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"strings"

	waBinary "wa-api/internal/wa-noise/binary"
	"wa-api/internal/wa-noise/proto/waE2E"
	"wa-api/internal/wa-noise/types"
)

func getTypeFromMessage(msg *waE2E.Message) string {
	switch {
	case msg.ViewOnceMessage != nil:
		return getTypeFromMessage(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return getTypeFromMessage(msg.ViewOnceMessageV2.Message)
	case msg.ViewOnceMessageV2Extension != nil:
		return getTypeFromMessage(msg.ViewOnceMessageV2Extension.Message)
	case msg.LottieStickerMessage != nil:
		return getTypeFromMessage(msg.LottieStickerMessage.Message)
	case msg.EphemeralMessage != nil:
		return getTypeFromMessage(msg.EphemeralMessage.Message)
	case msg.DocumentWithCaptionMessage != nil:
		return getTypeFromMessage(msg.DocumentWithCaptionMessage.Message)
	case msg.ReactionMessage != nil, msg.EncReactionMessage != nil:
		return "reaction"
	case msg.PollCreationMessage != nil, msg.PollUpdateMessage != nil:
		return "poll"
	case getMediaTypeFromMessage(msg) != "":
		return "media"
	case msg.Conversation != nil, msg.ExtendedTextMessage != nil, msg.ProtocolMessage != nil:
		return "text"
	default:
		return "text"
	}
}

func getMediaTypeFromMessage(msg *waE2E.Message) string {
	switch {
	case msg.ViewOnceMessage != nil:
		return getMediaTypeFromMessage(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return getMediaTypeFromMessage(msg.ViewOnceMessageV2.Message)
	case msg.ViewOnceMessageV2Extension != nil:
		return getMediaTypeFromMessage(msg.ViewOnceMessageV2Extension.Message)
	case msg.LottieStickerMessage != nil:
		return getMediaTypeFromMessage(msg.LottieStickerMessage.Message)
	case msg.EphemeralMessage != nil:
		return getMediaTypeFromMessage(msg.EphemeralMessage.Message)
	case msg.DocumentWithCaptionMessage != nil:
		return getMediaTypeFromMessage(msg.DocumentWithCaptionMessage.Message)
	case msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Title != nil:
		return "url"
	case msg.ImageMessage != nil:
		return "image"
	case msg.StickerMessage != nil:
		return "sticker"
	case msg.DocumentMessage != nil:
		return "document"
	case msg.AudioMessage != nil:
		if msg.AudioMessage.GetPTT() {
			return "ptt"
		} else {
			return "audio"
		}
	case msg.VideoMessage != nil:
		if msg.VideoMessage.GetGifPlayback() {
			return "gif"
		} else {
			return "video"
		}
	case msg.ContactMessage != nil:
		return "vcard"
	case msg.ContactsArrayMessage != nil:
		return "contact_array"
	case msg.ListMessage != nil:
		return "list"
	case msg.ListResponseMessage != nil:
		return "list_response"
	case msg.ButtonsResponseMessage != nil:
		return "buttons_response"
	case msg.OrderMessage != nil:
		return "order"
	case msg.ProductMessage != nil:
		return "product"
	case msg.InteractiveResponseMessage != nil:
		return "native_flow_response"
	default:
		return ""
	}
}

func getButtonTypeFromMessage(msg *waE2E.Message) string {
	switch {
	case msg.ViewOnceMessage != nil:
		return getButtonTypeFromMessage(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return getButtonTypeFromMessage(msg.ViewOnceMessageV2.Message)
	case msg.EphemeralMessage != nil:
		return getButtonTypeFromMessage(msg.EphemeralMessage.Message)
	case msg.ButtonsMessage != nil:
		return "buttons"
	case msg.ButtonsResponseMessage != nil:
		return "buttons_response"
	case msg.ListMessage != nil:
		return "list"
	case msg.ListResponseMessage != nil:
		return "list_response"
	case msg.InteractiveResponseMessage != nil:
		return "interactive_response"
	default:
		return ""
	}
}

func getButtonAttributes(msg *waE2E.Message) waBinary.Attrs {
	switch {
	case msg.ViewOnceMessage != nil:
		return getButtonAttributes(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return getButtonAttributes(msg.ViewOnceMessageV2.Message)
	case msg.EphemeralMessage != nil:
		return getButtonAttributes(msg.EphemeralMessage.Message)
	case msg.TemplateMessage != nil:
		return waBinary.Attrs{}
	case msg.ListMessage != nil:
		return waBinary.Attrs{
			"v":    "2",
			"type": strings.ToLower(waE2E.ListMessage_ListType_name[int32(msg.ListMessage.GetListType())]),
		}
	default:
		return waBinary.Attrs{}
	}
}

const RemoveReactionText = ""

func getEditAttribute(msg *waE2E.Message) types.EditAttribute {
	switch {
	case msg.EditedMessage != nil && msg.EditedMessage.Message != nil:
		return getEditAttribute(msg.EditedMessage.Message)
	case msg.ProtocolMessage != nil && msg.ProtocolMessage.GetKey() != nil:
		switch msg.ProtocolMessage.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			if msg.ProtocolMessage.GetKey().GetFromMe() {
				return types.EditAttributeSenderRevoke
			} else {
				return types.EditAttributeAdminRevoke
			}
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			if msg.ProtocolMessage.EditedMessage != nil {
				return types.EditAttributeMessageEdit
			}
		}
	case msg.ReactionMessage != nil && msg.ReactionMessage.GetText() == RemoveReactionText:
		return types.EditAttributeSenderRevoke
	case msg.KeepInChatMessage != nil && msg.KeepInChatMessage.GetKey().GetFromMe() && msg.KeepInChatMessage.GetKeepType() == waE2E.KeepType_UNDO_KEEP_FOR_ALL:
		return types.EditAttributeSenderRevoke
	case msg.PinInChatMessage != nil:
		return types.EditAttributePinInChat
	}
	return types.EditAttributeEmpty
}
