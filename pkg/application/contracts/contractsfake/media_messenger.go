package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DefaultSentImageMessageID é o ID que MediaMessenger devolve em
// MessageSendResult.ID sem SendImageFunc configurada.
const DefaultSentImageMessageID = "sent-image-message-id"

// MediaMessengerSendImageCall é uma chamada a SendImage.
type MediaMessengerSendImageCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.MediaPayload
	ID      string
}

// MediaMessenger é o fake de port.MediaMessenger.
type MediaMessenger struct {
	SessionGuard

	SendImageFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error)
	SendImageCalls []MediaMessengerSendImageCall
}

var _ port.MediaMessenger = (*MediaMessenger)(nil)

// SendImage implementa port.MediaMessenger.
func (f *MediaMessenger) SendImage(ctx context.Context, txtID string, target domain.JID, payload domain.MediaPayload, id string) (domain.MessageSendResult, error) {
	f.SendImageCalls = append(f.SendImageCalls, MediaMessengerSendImageCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ID: id})
	if f.SendImageFunc != nil {
		return f.SendImageFunc(ctx, txtID, target, payload, id)
	}
	return domain.MessageSendResult{ID: DefaultSentImageMessageID}, nil
}
