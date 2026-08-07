package msgattrs

import (
	armadillo "wa-api/internal/wa-noise/protocol/proto"
	"wa-api/internal/wa-noise/protocol/proto/waArmadilloApplication"
	"wa-api/internal/wa-noise/protocol/proto/waConsumerApplication"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

type MessageAttrs struct {
	Type        string
	MediaType   string
	Edit        types.EditAttribute
	DecryptFail events.DecryptFailMode
	PollType    string
}

func GetAttrsFromFBMessage(msg armadillo.MessageApplicationSub) (attrs MessageAttrs) {
	switch typedMsg := msg.(type) {
	case *waConsumerApplication.ConsumerApplication:
		return getAttrsFromFBConsumerMessage(typedMsg)
	case *waArmadilloApplication.Armadillo:
		attrs.Type = "media"
		attrs.MediaType = "document"
	default:
		attrs.Type = "text"
	}
	return
}

func getAttrsFromFBConsumerMessage(msg *waConsumerApplication.ConsumerApplication) (attrs MessageAttrs) {
	switch payload := msg.GetPayload().GetPayload().(type) {
	case *waConsumerApplication.ConsumerApplication_Payload_Content:
		switch content := payload.Content.GetContent().(type) {
		case *waConsumerApplication.ConsumerApplication_Content_MessageText,
			*waConsumerApplication.ConsumerApplication_Content_ExtendedTextMessage:
			attrs.Type = "text"
		case *waConsumerApplication.ConsumerApplication_Content_ImageMessage:
			attrs.MediaType = "image"
		case *waConsumerApplication.ConsumerApplication_Content_StickerMessage:
			attrs.MediaType = "sticker"
		case *waConsumerApplication.ConsumerApplication_Content_ViewOnceMessage:
			switch content.ViewOnceMessage.GetViewOnceContent().(type) {
			case *waConsumerApplication.ConsumerApplication_ViewOnceMessage_ImageMessage:
				attrs.MediaType = "image"
			case *waConsumerApplication.ConsumerApplication_ViewOnceMessage_VideoMessage:
				attrs.MediaType = "video"
			}
		case *waConsumerApplication.ConsumerApplication_Content_DocumentMessage:
			attrs.MediaType = "document"
		case *waConsumerApplication.ConsumerApplication_Content_AudioMessage:
			if content.AudioMessage.GetPTT() {
				attrs.MediaType = "ptt"
			} else {
				attrs.MediaType = "audio"
			}
		case *waConsumerApplication.ConsumerApplication_Content_VideoMessage:
			// TODO gifPlayback?
			attrs.MediaType = "video"
		case *waConsumerApplication.ConsumerApplication_Content_LocationMessage:
			attrs.MediaType = "location"
		case *waConsumerApplication.ConsumerApplication_Content_LiveLocationMessage:
			attrs.MediaType = "location"
		case *waConsumerApplication.ConsumerApplication_Content_ContactMessage:
			attrs.MediaType = "vcard"
		case *waConsumerApplication.ConsumerApplication_Content_ContactsArrayMessage:
			attrs.MediaType = "contact_array"
		case *waConsumerApplication.ConsumerApplication_Content_PollCreationMessage:
			attrs.PollType = "creation"
			attrs.Type = "poll"
		case *waConsumerApplication.ConsumerApplication_Content_PollUpdateMessage:
			attrs.PollType = "vote"
			attrs.Type = "poll"
			attrs.DecryptFail = events.DecryptFailHide
		case *waConsumerApplication.ConsumerApplication_Content_ReactionMessage:
			attrs.Type = "reaction"
			attrs.DecryptFail = events.DecryptFailHide
		case *waConsumerApplication.ConsumerApplication_Content_EditMessage:
			attrs.Edit = types.EditAttributeMessageEdit
			attrs.DecryptFail = events.DecryptFailHide
		}
		if attrs.MediaType != "" && attrs.Type == "" {
			attrs.Type = "media"
		}
	case *waConsumerApplication.ConsumerApplication_Payload_ApplicationData:
		switch content := payload.ApplicationData.GetApplicationContent().(type) {
		case *waConsumerApplication.ConsumerApplication_ApplicationData_Revoke:
			if content.Revoke.GetKey().GetFromMe() {
				attrs.Edit = types.EditAttributeSenderRevoke
			} else {
				attrs.Edit = types.EditAttributeAdminRevoke
			}
			attrs.DecryptFail = events.DecryptFailHide
		}
	case *waConsumerApplication.ConsumerApplication_Payload_Signal:
	case *waConsumerApplication.ConsumerApplication_Payload_SubProtocol:
	}
	if attrs.Type == "" {
		attrs.Type = "text"
	}
	return
}
