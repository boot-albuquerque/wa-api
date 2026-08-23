package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DefaultSentImageMessageID é o ID que MediaMessenger devolve em
// MessageSendResult.ID sem SendImageFunc configurada.
const DefaultSentImageMessageID = "sent-image-message-id"

// DefaultSentDocumentMessageID é o ID que MediaMessenger devolve em
// MessageSendResult.ID sem SendDocumentFunc configurada.
const DefaultSentDocumentMessageID = "sent-document-message-id"

// DefaultSentAudioMessageID é o ID que MediaMessenger devolve em
// MessageSendResult.ID sem SendAudioFunc configurada.
const DefaultSentAudioMessageID = "sent-audio-message-id"

// DefaultSentVideoMessageID é o ID que MediaMessenger devolve em
// MessageSendResult.ID sem SendVideoFunc configurada.
const DefaultSentVideoMessageID = "sent-video-message-id"

// DefaultSentStickerMessageID é o ID que MediaMessenger devolve em
// MessageSendResult.ID sem SendStickerFunc configurada.
const DefaultSentStickerMessageID = "sent-sticker-message-id"

// MediaMessengerSendImageCall é uma chamada a SendImage.
type MediaMessengerSendImageCall struct {
	Ctx          context.Context
	TxtID        string
	Target       domain.JID
	Payload      domain.MediaPayload
	ReplyTo      *domain.ReplyContext
	MentionedJID []string
	ID           string
}

// MediaMessengerSendDocumentCall é uma chamada a SendDocument.
type MediaMessengerSendDocumentCall struct {
	Ctx          context.Context
	TxtID        string
	Target       domain.JID
	Payload      domain.MediaPayload
	ReplyTo      *domain.ReplyContext
	MentionedJID []string
	ID           string
}

// MediaMessengerSendAudioCall é uma chamada a SendAudio.
type MediaMessengerSendAudioCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.AudioPayload
	ReplyTo *domain.ReplyContext
	ID      string
}

// MediaMessengerSendVideoCall é uma chamada a SendVideo.
type MediaMessengerSendVideoCall struct {
	Ctx          context.Context
	TxtID        string
	Target       domain.JID
	Payload      domain.MediaPayload
	ReplyTo      *domain.ReplyContext
	MentionedJID []string
	ID           string
}

// MediaMessengerSendStickerCall é uma chamada a SendSticker.
type MediaMessengerSendStickerCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.MediaPayload
	ReplyTo *domain.ReplyContext
	ID      string
}

// MediaMessenger é o fake de port.MediaMessenger.
type MediaMessenger struct {
	SessionGuard

	SendImageFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)
	SendImageCalls []MediaMessengerSendImageCall

	SendDocumentFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)
	SendDocumentCalls []MediaMessengerSendDocumentCall

	SendAudioFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.AudioPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
	SendAudioCalls []MediaMessengerSendAudioCall

	SendVideoFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)
	SendVideoCalls []MediaMessengerSendVideoCall

	SendStickerFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
	SendStickerCalls []MediaMessengerSendStickerCall
}

var _ port.MediaMessenger = (*MediaMessenger)(nil)

// SendImage implementa port.MediaMessenger.
func (f *MediaMessenger) SendImage(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error) {
	f.SendImageCalls = append(f.SendImageCalls, MediaMessengerSendImageCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, MentionedJID: mentionedJID, ID: id})
	if f.SendImageFunc != nil {
		return f.SendImageFunc(ctx, txtID, target, payload, replyTo, mentionedJID, id)
	}
	return domain.MessageSendResult{ID: DefaultSentImageMessageID}, nil
}

// SendDocument implementa port.MediaMessenger.
func (f *MediaMessenger) SendDocument(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error) {
	f.SendDocumentCalls = append(f.SendDocumentCalls, MediaMessengerSendDocumentCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, MentionedJID: mentionedJID, ID: id})
	if f.SendDocumentFunc != nil {
		return f.SendDocumentFunc(ctx, txtID, target, payload, replyTo, mentionedJID, id)
	}
	return domain.MessageSendResult{ID: DefaultSentDocumentMessageID}, nil
}

// SendAudio implementa port.MediaMessenger.
func (f *MediaMessenger) SendAudio(ctx context.Context, txtID string, target domain.JID, payload domain.AudioPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	f.SendAudioCalls = append(f.SendAudioCalls, MediaMessengerSendAudioCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, ID: id})
	if f.SendAudioFunc != nil {
		return f.SendAudioFunc(ctx, txtID, target, payload, replyTo, id)
	}
	return domain.MessageSendResult{ID: DefaultSentAudioMessageID}, nil
}

// SendVideo implementa port.MediaMessenger.
func (f *MediaMessenger) SendVideo(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error) {
	f.SendVideoCalls = append(f.SendVideoCalls, MediaMessengerSendVideoCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, MentionedJID: mentionedJID, ID: id})
	if f.SendVideoFunc != nil {
		return f.SendVideoFunc(ctx, txtID, target, payload, replyTo, mentionedJID, id)
	}
	return domain.MessageSendResult{ID: DefaultSentVideoMessageID}, nil
}

// SendSticker implementa port.MediaMessenger.
func (f *MediaMessenger) SendSticker(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	f.SendStickerCalls = append(f.SendStickerCalls, MediaMessengerSendStickerCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, ID: id})
	if f.SendStickerFunc != nil {
		return f.SendStickerFunc(ctx, txtID, target, payload, replyTo, id)
	}
	return domain.MessageSendResult{ID: DefaultSentStickerMessageID}, nil
}
