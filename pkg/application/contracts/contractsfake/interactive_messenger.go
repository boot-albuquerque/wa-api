package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DefaultSentButtonsMessageID é o ID que InteractiveMessenger devolve em
// MessageSendResult.ID sem SendButtonsFunc configurada.
const DefaultSentButtonsMessageID = "sent-buttons-message-id"

// InteractiveMessengerSendButtonsCall é uma chamada a SendButtons.
type InteractiveMessengerSendButtonsCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.ButtonsPayload
	ID      string
}

// InteractiveMessenger é o fake de port.InteractiveMessenger.
type InteractiveMessenger struct {
	SessionGuard

	SendButtonsFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.ButtonsPayload, id string) (domain.MessageSendResult, error)
	SendButtonsCalls []InteractiveMessengerSendButtonsCall
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
