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

// SimpleMessengerSendLocationCall é uma chamada a SendLocation.
type SimpleMessengerSendLocationCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.LocationPayload
	ID      string
}

// SimpleMessengerSendContactCall é uma chamada a SendContact.
type SimpleMessengerSendContactCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.ContactPayload
	ID      string
}

// SimpleMessenger é o fake de port.SimpleMessenger.
type SimpleMessenger struct {
	SessionGuard

	SendLocationFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.LocationPayload, id string) (domain.MessageSendResult, error)
	SendLocationCalls []SimpleMessengerSendLocationCall

	SendContactFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.ContactPayload, id string) (domain.MessageSendResult, error)
	SendContactCalls []SimpleMessengerSendContactCall
}

var _ port.SimpleMessenger = (*SimpleMessenger)(nil)

// SendLocation implementa port.SimpleMessenger.
func (f *SimpleMessenger) SendLocation(ctx context.Context, txtID string, target domain.JID, payload domain.LocationPayload, id string) (domain.MessageSendResult, error) {
	f.SendLocationCalls = append(f.SendLocationCalls, SimpleMessengerSendLocationCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ID: id})
	if f.SendLocationFunc != nil {
		return f.SendLocationFunc(ctx, txtID, target, payload, id)
	}
	return domain.MessageSendResult{ID: DefaultSentLocationMessageID}, nil
}

// SendContact implementa port.SimpleMessenger.
func (f *SimpleMessenger) SendContact(ctx context.Context, txtID string, target domain.JID, payload domain.ContactPayload, id string) (domain.MessageSendResult, error) {
	f.SendContactCalls = append(f.SendContactCalls, SimpleMessengerSendContactCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ID: id})
	if f.SendContactFunc != nil {
		return f.SendContactFunc(ctx, txtID, target, payload, id)
	}
	return domain.MessageSendResult{ID: DefaultSentContactMessageID}, nil
}
