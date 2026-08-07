package msgattrs

import (
	"strings"

	waBinary "wa-api/internal/wa-noise/protocol/binary"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

func GetTypeFromMessage(msg *waE2E.Message) string {
	switch {
	case msg.ViewOnceMessage != nil:
		return GetTypeFromMessage(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return GetTypeFromMessage(msg.ViewOnceMessageV2.Message)
	case msg.ViewOnceMessageV2Extension != nil:
		return GetTypeFromMessage(msg.ViewOnceMessageV2Extension.Message)
	case msg.LottieStickerMessage != nil:
		return GetTypeFromMessage(msg.LottieStickerMessage.Message)
	case msg.EphemeralMessage != nil:
		return GetTypeFromMessage(msg.EphemeralMessage.Message)
	case msg.DocumentWithCaptionMessage != nil:
		return GetTypeFromMessage(msg.DocumentWithCaptionMessage.Message)
	case msg.ReactionMessage != nil, msg.EncReactionMessage != nil:
		return "reaction"
	case msg.PollCreationMessage != nil, msg.PollUpdateMessage != nil:
		return "poll"
	case GetMediaTypeFromMessage(msg) != "":
		return "media"
	case msg.Conversation != nil, msg.ExtendedTextMessage != nil, msg.ProtocolMessage != nil:
		return "text"
	default:
		return "text"
	}
}

func GetMediaTypeFromMessage(msg *waE2E.Message) string {
	switch {
	case msg.ViewOnceMessage != nil:
		return GetMediaTypeFromMessage(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return GetMediaTypeFromMessage(msg.ViewOnceMessageV2.Message)
	case msg.ViewOnceMessageV2Extension != nil:
		return GetMediaTypeFromMessage(msg.ViewOnceMessageV2Extension.Message)
	case msg.LottieStickerMessage != nil:
		return GetMediaTypeFromMessage(msg.LottieStickerMessage.Message)
	case msg.EphemeralMessage != nil:
		return GetMediaTypeFromMessage(msg.EphemeralMessage.Message)
	case msg.DocumentWithCaptionMessage != nil:
		return GetMediaTypeFromMessage(msg.DocumentWithCaptionMessage.Message)
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

func GetButtonTypeFromMessage(msg *waE2E.Message) string {
	switch {
	case msg.ViewOnceMessage != nil:
		return GetButtonTypeFromMessage(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return GetButtonTypeFromMessage(msg.ViewOnceMessageV2.Message)
	case msg.EphemeralMessage != nil:
		return GetButtonTypeFromMessage(msg.EphemeralMessage.Message)
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

func GetButtonAttributes(msg *waE2E.Message) waBinary.Attrs {
	switch {
	case msg.ViewOnceMessage != nil:
		return GetButtonAttributes(msg.ViewOnceMessage.Message)
	case msg.ViewOnceMessageV2 != nil:
		return GetButtonAttributes(msg.ViewOnceMessageV2.Message)
	case msg.EphemeralMessage != nil:
		return GetButtonAttributes(msg.EphemeralMessage.Message)
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

func GetEditAttribute(msg *waE2E.Message) types.EditAttribute {
	switch {
	case msg.EditedMessage != nil && msg.EditedMessage.Message != nil:
		return GetEditAttribute(msg.EditedMessage.Message)
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
