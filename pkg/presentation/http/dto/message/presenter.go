package message

import "wa-api/pkg/domain"

// Presenters are hand-written, one function per domain type, field by field.
//
// No reflection, no generic struct copier, no `json` round-trip. The property
// being bought is a COMPILE ERROR: rename a field in domain.SendMessageResult
// and this file stops building. A reflection-based mapper would keep building
// and would silently drop the key from every response, which is the exact
// failure the DTO layer exists to prevent.
//
// There is one function per RESULT TYPE and not one for all of them, even
// though fourteen of the fifteen shapes are identical today. Collapsing them
// behind an interface or a type parameter would buy back exactly the property
// above: the day one capability grows a field, the compiler has to point at
// the presenter that does not carry it.

// presentTimestamp renders a Unix instant, or null when the engine reported
// none.
//
// Zero is null and not 0: on the wire 0 is 1970-01-01T00:00:00Z, a real date,
// and a client that parses it gets a message sent before WhatsApp existed
// instead of a missing field. Same reasoning as presentTime in the group
// family, one type down.
func presentTimestamp(unix int64) *int64 {
	if unix == 0 {
		return nil
	}
	v := unix
	return &v
}

// presentOptionalString renders a string that is absent-or-present, where ""
// is not a legitimate value. Used only for the audio caption pair.
func presentOptionalString(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

// PresentSendMessage maps POST /chat/send/text.
func PresentSendMessage(r *domain.SendMessageResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendImage maps POST /chat/send/image.
func PresentSendImage(r *domain.SendImageResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendDocument maps POST /chat/send/document.
func PresentSendDocument(r *domain.SendDocumentResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendAudio maps POST /chat/send/audio, including the caption pair.
func PresentSendAudio(r *domain.SendAudioResult) *SendAudioResponse {
	if r == nil {
		return nil
	}
	return &SendAudioResponse{
		MessageID:        r.MessageID,
		Timestamp:        presentTimestamp(r.Timestamp),
		Status:           r.Status,
		CaptionMessageID: presentOptionalString(r.CaptionMessageID),
		CaptionStatus:    presentOptionalString(r.CaptionStatus),
	}
}

// PresentSendSticker maps POST /chat/send/sticker.
func PresentSendSticker(r *domain.SendStickerResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendVideo maps POST /chat/send/video.
func PresentSendVideo(r *domain.SendVideoResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendContact maps POST /chat/send/contact.
func PresentSendContact(r *domain.SendContactResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendLocation maps POST /chat/send/location.
func PresentSendLocation(r *domain.SendLocationResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendButtons maps POST /chat/send/buttons.
func PresentSendButtons(r *domain.SendButtonsResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendCarousel maps POST /chat/send/carousel.
func PresentSendCarousel(r *domain.SendCarouselResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendList maps POST /chat/send/list.
func PresentSendList(r *domain.SendListResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendPoll maps POST /chat/send/poll.
func PresentSendPoll(r *domain.SendPollResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendPollVote maps POST /chat/send/pollvote.
func PresentSendPollVote(r *domain.SendPollVoteResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendTemplate maps POST /chat/send/template.
func PresentSendTemplate(r *domain.SendTemplateResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendForward maps POST /chat/send/forward.
func PresentSendForward(r *domain.SendForwardResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentDeleteMessage maps POST /chat/delete.
func PresentDeleteMessage(r *domain.DeleteMessageResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendEditMessage maps POST /chat/send/edit.
func PresentSendEditMessage(r *domain.SendEditMessageResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}

// PresentSendReaction maps POST /chat/react.
func PresentSendReaction(r *domain.SendReactionResult) *SendResponse {
	if r == nil {
		return nil
	}
	return &SendResponse{
		MessageID: r.MessageID,
		Timestamp: presentTimestamp(r.Timestamp),
		Status:    r.Status,
	}
}
