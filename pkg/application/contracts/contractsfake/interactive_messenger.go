package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DefaultSentButtonsMessageID é o ID que InteractiveMessenger devolve em
// MessageSendResult.ID sem SendButtonsFunc configurada.
const DefaultSentButtonsMessageID = "sent-buttons-message-id"

// DefaultSentCarouselMessageID é o ID que InteractiveMessenger devolve em
// MessageSendResult.ID sem SendCarouselFunc configurada.
const DefaultSentCarouselMessageID = "sent-carousel-message-id"

// InteractiveMessengerSendButtonsCall é uma chamada a SendButtons.
type InteractiveMessengerSendButtonsCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.ButtonsPayload
	ID      string
}

// InteractiveMessengerSendCarouselCall é uma chamada a SendCarousel.
type InteractiveMessengerSendCarouselCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.CarouselPayload
	ID      string
}

// InteractiveMessenger é o fake de port.InteractiveMessenger.
type InteractiveMessenger struct {
	SessionGuard

	SendButtonsFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.ButtonsPayload, id string) (domain.MessageSendResult, error)
	SendButtonsCalls []InteractiveMessengerSendButtonsCall

	SendCarouselFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.CarouselPayload, id string) (domain.MessageSendResult, error)
	SendCarouselCalls []InteractiveMessengerSendCarouselCall
}

var _ port.InteractiveMessenger = (*InteractiveMessenger)(nil)

// SendButtons implementa port.InteractiveMessenger.
func (f *InteractiveMessenger) SendButtons(ctx context.Context, txtID string, target domain.JID, payload domain.ButtonsPayload, id string) (domain.MessageSendResult, error) {
	f.SendButtonsCalls = append(f.SendButtonsCalls, InteractiveMessengerSendButtonsCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ID: id})
	if f.SendButtonsFunc != nil {
		return f.SendButtonsFunc(ctx, txtID, target, payload, id)
	}
	return domain.MessageSendResult{ID: DefaultSentButtonsMessageID}, nil
}

// SendCarousel implementa port.InteractiveMessenger.
func (f *InteractiveMessenger) SendCarousel(ctx context.Context, txtID string, target domain.JID, payload domain.CarouselPayload, id string) (domain.MessageSendResult, error) {
	f.SendCarouselCalls = append(f.SendCarouselCalls, InteractiveMessengerSendCarouselCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ID: id})
	if f.SendCarouselFunc != nil {
		return f.SendCarouselFunc(ctx, txtID, target, payload, id)
	}
	return domain.MessageSendResult{ID: DefaultSentCarouselMessageID}, nil
}
