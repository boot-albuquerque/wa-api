package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ForwardedMessageSenderCall records one call to SendForwardedMessage.
type ForwardedMessageSenderCall struct {
	TxtID    string
	Target   domain.JID
	DataJSON string
	ID       string
}

// ForwardedMessageSender is the fake of port.ForwardedMessageSender.
type ForwardedMessageSender struct {
	SessionGuard

	SendForwardedMessageFunc  func(ctx context.Context, txtID string, target domain.JID, dataJSON string, id string) (domain.MessageSendResult, error)
	SendForwardedMessageCalls []ForwardedMessageSenderCall
}

var _ port.ForwardedMessageSender = (*ForwardedMessageSender)(nil)

// SendForwardedMessage implements port.ForwardedMessageSender.
func (f *ForwardedMessageSender) SendForwardedMessage(ctx context.Context, txtID string, target domain.JID, dataJSON string, id string) (domain.MessageSendResult, error) {
	f.SendForwardedMessageCalls = append(f.SendForwardedMessageCalls, ForwardedMessageSenderCall{
		TxtID: txtID, Target: target, DataJSON: dataJSON, ID: id,
	})
	if f.SendForwardedMessageFunc != nil {
		return f.SendForwardedMessageFunc(ctx, txtID, target, dataJSON, id)
	}
	return domain.MessageSendResult{ID: DefaultSentMessageID}, nil
}
