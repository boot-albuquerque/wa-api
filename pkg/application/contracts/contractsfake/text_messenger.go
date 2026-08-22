package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// DefaultSentMessageID é o ID que TextMessenger devolve em
// MessageSendResult.ID sem SendTextFunc configurada — distinto de
// DefaultMessageID (o gerador de ID) para que um teste que confunda "ID
// gerado antes do envio" com "ID que o SDK realmente usou" quebre.
const DefaultSentMessageID = "sent-message-id"

// TextMessengerSendTextCall é uma chamada a SendText.
type TextMessengerSendTextCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Text    string
	Preview *domain.LinkPreviewData
	ReplyTo *domain.ReplyContext
	ID      string
}

// TextMessenger é o fake de port.TextMessenger.
type TextMessenger struct {
	SessionGuard

	SendTextFunc  func(ctx context.Context, txtID string, target domain.JID, text string, preview *domain.LinkPreviewData, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error)
	SendTextCalls []TextMessengerSendTextCall
}

var _ port.TextMessenger = (*TextMessenger)(nil)

// SendText implementa port.TextMessenger.
func (f *TextMessenger) SendText(ctx context.Context, txtID string, target domain.JID, text string, preview *domain.LinkPreviewData, replyTo *domain.ReplyContext, id string) (domain.MessageSendResult, error) {
	f.SendTextCalls = append(f.SendTextCalls, TextMessengerSendTextCall{Ctx: ctx, TxtID: txtID, Target: target, Text: text, Preview: preview, ReplyTo: replyTo, ID: id})
	if f.SendTextFunc != nil {
		return f.SendTextFunc(ctx, txtID, target, text, preview, replyTo, id)
	}
	return domain.MessageSendResult{ID: DefaultSentMessageID}, nil
}
