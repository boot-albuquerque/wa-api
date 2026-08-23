package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DefaultSentLocationMessageID é o ID que SimpleMessenger devolve em
// MessageSendResult.ID sem SendLocationFunc configurada.
const DefaultSentLocationMessageID = "sent-location-message-id"

// DefaultSentContactMessageID é o ID que SimpleMessenger devolve em
// MessageSendResult.ID sem SendContactFunc configurada.
const DefaultSentContactMessageID = "sent-contact-message-id"

// DefaultSentPollMessageID é o ID que SimpleMessenger devolve em
// MessageSendResult.ID sem SendPollFunc configurada.
const DefaultSentPollMessageID = "sent-poll-message-id"

// SimpleMessengerSendLocationCall é uma chamada a SendLocation.
type SimpleMessengerSendLocationCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.LocationPayload
	ReplyTo *domain.ReplyContext
	ID      string
}

// SimpleMessengerSendContactCall é uma chamada a SendContact.
type SimpleMessengerSendContactCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.ContactPayload
	ReplyTo *domain.ReplyContext
	ID      string
}

// DefaultSentTemplateMessageID é o ID que SimpleMessenger devolve em
// MessageSendResult.ID sem SendTemplateFunc configurada.
const DefaultSentTemplateMessageID = "sent-template-message-id"

// SimpleMessengerSendPollCall é uma chamada a SendPoll.
type SimpleMessengerSendPollCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.PollPayload
	ReplyTo *domain.ReplyContext
	ID      string
}

// SimpleMessengerSendTemplateCall é uma chamada a SendTemplate.
type SimpleMessengerSendTemplateCall struct {
	Ctx          context.Context
	TxtID        string
	Target       domain.JID
	Payload      domain.TemplatePayload
	ReplyTo      *domain.ReplyContext
	MentionedJID []string
	ID           string
}

// DefaultSentListMessageID é o ID que SimpleMessenger devolve em
// MessageSendResult.ID sem SendListFunc configurada.
const DefaultSentListMessageID = "sent-list-message-id"

// SimpleMessengerSendListCall é uma chamada a SendList.
type SimpleMessengerSendListCall struct {
	Ctx          context.Context
	TxtID        string
	Target       domain.JID
	Payload      domain.ListPayload
	ReplyTo      *domain.ReplyContext
	MentionedJID []string
	ID           string
}

// SimpleMessenger é o fake de port.SimpleMessenger.
type SimpleMessenger struct {
	SessionGuard

	SendLocationFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.LocationPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
	SendLocationCalls []SimpleMessengerSendLocationCall

	SendContactFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.ContactPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
	SendContactCalls []SimpleMessengerSendContactCall

	SendPollFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.PollPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
	SendPollCalls []SimpleMessengerSendPollCall

	SendTemplateFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.TemplatePayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)
	SendTemplateCalls []SimpleMessengerSendTemplateCall

	SendListFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.ListPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error)
	SendListCalls []SimpleMessengerSendListCall
}

var _ port.SimpleMessenger = (*SimpleMessenger)(nil)

// SendLocation implementa port.SimpleMessenger.
func (f *SimpleMessenger) SendLocation(ctx context.Context, txtID string, target domain.JID, payload domain.LocationPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	f.SendLocationCalls = append(f.SendLocationCalls, SimpleMessengerSendLocationCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, ID: id})
	if f.SendLocationFunc != nil {
		return f.SendLocationFunc(ctx, txtID, target, payload, replyTo, id)
	}
	return domain.MessageSendResult{ID: DefaultSentLocationMessageID}, nil
}

// SendContact implementa port.SimpleMessenger.
func (f *SimpleMessenger) SendContact(ctx context.Context, txtID string, target domain.JID, payload domain.ContactPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	f.SendContactCalls = append(f.SendContactCalls, SimpleMessengerSendContactCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, ID: id})
	if f.SendContactFunc != nil {
		return f.SendContactFunc(ctx, txtID, target, payload, replyTo, id)
	}
	return domain.MessageSendResult{ID: DefaultSentContactMessageID}, nil
}

// SendPoll implementa port.SimpleMessenger.
func (f *SimpleMessenger) SendPoll(ctx context.Context, txtID string, target domain.JID, payload domain.PollPayload, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	f.SendPollCalls = append(f.SendPollCalls, SimpleMessengerSendPollCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, ID: id})
	if f.SendPollFunc != nil {
		return f.SendPollFunc(ctx, txtID, target, payload, replyTo, id)
	}
	return domain.MessageSendResult{ID: DefaultSentPollMessageID}, nil
}

// SendTemplate implementa port.SimpleMessenger.
func (f *SimpleMessenger) SendTemplate(ctx context.Context, txtID string, target domain.JID, payload domain.TemplatePayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error) {
	f.SendTemplateCalls = append(f.SendTemplateCalls, SimpleMessengerSendTemplateCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, MentionedJID: mentionedJID, ID: id})
	if f.SendTemplateFunc != nil {
		return f.SendTemplateFunc(ctx, txtID, target, payload, replyTo, mentionedJID, id)
	}
	return domain.MessageSendResult{ID: DefaultSentTemplateMessageID}, nil
}

// SendList implementa port.SimpleMessenger.
func (f *SimpleMessenger) SendList(ctx context.Context, txtID string, target domain.JID, payload domain.ListPayload, replyTo *domain.ReplyContext, mentionedJID []string, id string) (domain.MessageSendResult, error) {
	f.SendListCalls = append(f.SendListCalls, SimpleMessengerSendListCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ReplyTo: replyTo, MentionedJID: mentionedJID, ID: id})
	if f.SendListFunc != nil {
		return f.SendListFunc(ctx, txtID, target, payload, replyTo, mentionedJID, id)
	}
	return domain.MessageSendResult{ID: DefaultSentListMessageID}, nil
}
