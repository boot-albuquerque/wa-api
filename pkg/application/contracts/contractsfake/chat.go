package contractsfake

import (
	"context"
	"time"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// --- JIDResolver -------------------------------------------------------

// JIDResolverResolveJIDCall é uma chamada a ResolveJID.
type JIDResolverResolveJIDCall struct {
	Ctx context.Context
	Raw string
}

// JIDResolverResolveQualifiedJIDCall é uma chamada a ResolveQualifiedJID.
type JIDResolverResolveQualifiedJIDCall struct {
	Ctx context.Context
	Raw string
}

// JIDResolver é o fake de port.JIDResolver.
//
// O zero-value NÃO é neutro por acaso: sem Func configurada, ambos os métodos
// devolvem domain.JID(raw) e nil, isto é, tratam a entrada como já canônica.
// É o comportamento que deixa o teste focar no use case em vez de na
// gramática de JID do SDK.
type JIDResolver struct {
	ResolveJIDFunc  func(ctx context.Context, raw string) (domain.JID, error)
	ResolveJIDCalls []JIDResolverResolveJIDCall

	ResolveQualifiedJIDFunc  func(ctx context.Context, raw string) (domain.JID, error)
	ResolveQualifiedJIDCalls []JIDResolverResolveQualifiedJIDCall
}

var _ port.JIDResolver = (*JIDResolver)(nil)

// ResolveJID implementa port.JIDResolver.
func (f *JIDResolver) ResolveJID(ctx context.Context, raw string) (domain.JID, error) {
	f.ResolveJIDCalls = append(f.ResolveJIDCalls, JIDResolverResolveJIDCall{Ctx: ctx, Raw: raw})
	if f.ResolveJIDFunc != nil {
		return f.ResolveJIDFunc(ctx, raw)
	}
	return domain.JID(raw), nil
}

// ResolveQualifiedJID implementa port.JIDResolver.
func (f *JIDResolver) ResolveQualifiedJID(ctx context.Context, raw string) (domain.JID, error) {
	f.ResolveQualifiedJIDCalls = append(f.ResolveQualifiedJIDCalls, JIDResolverResolveQualifiedJIDCall{Ctx: ctx, Raw: raw})
	if f.ResolveQualifiedJIDFunc != nil {
		return f.ResolveQualifiedJIDFunc(ctx, raw)
	}
	return domain.JID(raw), nil
}

// --- PresenceController ------------------------------------------------

// PresenceControllerSendPresenceCall é uma chamada a SendPresence.
type PresenceControllerSendPresenceCall struct {
	Ctx      context.Context
	TxtID    string
	Presence domain.PresenceType
}

// PresenceControllerSendChatPresenceCall é uma chamada a SendChatPresence.
type PresenceControllerSendChatPresenceCall struct {
	Ctx   context.Context
	TxtID string
	Chat  domain.JID
	State string
	Media string
}

// PresenceControllerSubscribePresenceCall é uma chamada a SubscribePresence.
type PresenceControllerSubscribePresenceCall struct {
	Ctx    context.Context
	TxtID  string
	Target domain.JID
}

// PresenceController é o fake de port.PresenceController.
type PresenceController struct {
	SessionGuard

	SendPresenceFunc  func(ctx context.Context, txtID string, presence domain.PresenceType) error
	SendPresenceCalls []PresenceControllerSendPresenceCall

	SendChatPresenceFunc  func(ctx context.Context, txtID string, chat domain.JID, state, media string) error
	SendChatPresenceCalls []PresenceControllerSendChatPresenceCall

	SubscribePresenceFunc  func(ctx context.Context, txtID string, target domain.JID) error
	SubscribePresenceCalls []PresenceControllerSubscribePresenceCall
}

var _ port.PresenceController = (*PresenceController)(nil)

// SendPresence implementa port.PresenceController.
func (f *PresenceController) SendPresence(ctx context.Context, txtID string, presence domain.PresenceType) error {
	f.SendPresenceCalls = append(f.SendPresenceCalls, PresenceControllerSendPresenceCall{Ctx: ctx, TxtID: txtID, Presence: presence})
	if f.SendPresenceFunc != nil {
		return f.SendPresenceFunc(ctx, txtID, presence)
	}
	return nil
}

// SendChatPresence implementa port.PresenceController.
func (f *PresenceController) SendChatPresence(ctx context.Context, txtID string, chat domain.JID, state, media string) error {
	f.SendChatPresenceCalls = append(f.SendChatPresenceCalls, PresenceControllerSendChatPresenceCall{Ctx: ctx, TxtID: txtID, Chat: chat, State: state, Media: media})
	if f.SendChatPresenceFunc != nil {
		return f.SendChatPresenceFunc(ctx, txtID, chat, state, media)
	}
	return nil
}

// SubscribePresence implementa port.PresenceController.
func (f *PresenceController) SubscribePresence(ctx context.Context, txtID string, target domain.JID) error {
	f.SubscribePresenceCalls = append(f.SubscribePresenceCalls, PresenceControllerSubscribePresenceCall{Ctx: ctx, TxtID: txtID, Target: target})
	if f.SubscribePresenceFunc != nil {
		return f.SubscribePresenceFunc(ctx, txtID, target)
	}
	return nil
}

// --- ChatMessenger -----------------------------------------------------

// ChatMessengerMarkReadCall é uma chamada a MarkRead.
type ChatMessengerMarkReadCall struct {
	Ctx    context.Context
	TxtID  string
	IDs    []string
	At     time.Time
	Chat   domain.JID
	Sender domain.JID
}

// ChatMessengerSendReactionCall é uma chamada a SendReaction.
type ChatMessengerSendReactionCall struct {
	Ctx      context.Context
	TxtID    string
	Target   domain.JID
	Reaction domain.Reaction
}

// ChatMessenger é o fake de port.ChatMessenger.
type ChatMessenger struct {
	SessionGuard

	MarkReadFunc  func(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error
	MarkReadCalls []ChatMessengerMarkReadCall

	SendReactionFunc  func(ctx context.Context, txtID string, target domain.JID, reaction domain.Reaction) (domain.MessageSendResult, error)
	SendReactionCalls []ChatMessengerSendReactionCall
}

var _ port.ChatMessenger = (*ChatMessenger)(nil)

// MarkRead implementa port.ChatMessenger.
func (f *ChatMessenger) MarkRead(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error {
	f.MarkReadCalls = append(f.MarkReadCalls, ChatMessengerMarkReadCall{Ctx: ctx, TxtID: txtID, IDs: ids, At: at, Chat: chat, Sender: sender})
	if f.MarkReadFunc != nil {
		return f.MarkReadFunc(ctx, txtID, ids, at, chat, sender)
	}
	return nil
}

// SendReaction implementa port.ChatMessenger.
func (f *ChatMessenger) SendReaction(ctx context.Context, txtID string, target domain.JID, reaction domain.Reaction) (domain.MessageSendResult, error) {
	f.SendReactionCalls = append(f.SendReactionCalls, ChatMessengerSendReactionCall{Ctx: ctx, TxtID: txtID, Target: target, Reaction: reaction})
	if f.SendReactionFunc != nil {
		return f.SendReactionFunc(ctx, txtID, target, reaction)
	}
	return domain.MessageSendResult{}, nil
}

// --- ChatOperations ----------------------------------------------------

// ChatOperationsArchiveChatCall é uma chamada a ArchiveChat.
type ChatOperationsArchiveChatCall struct {
	Ctx     context.Context
	TxtID   string
	Chat    domain.JID
	Archive bool
}

// ChatOperationsRejectCallCall é uma chamada a RejectCall.
type ChatOperationsRejectCallCall struct {
	Ctx    context.Context
	TxtID  string
	From   domain.JID
	CallID string
}

// ChatOperationsRequestUnavailableMessageCall é uma chamada a
// RequestUnavailableMessage.
type ChatOperationsRequestUnavailableMessageCall struct {
	Ctx       context.Context
	TxtID     string
	Chat      domain.JID
	Sender    domain.JID
	MessageID string
}

// ChatOperations é o fake de port.ChatOperations.
type ChatOperations struct {
	SessionGuard

	ArchiveChatFunc  func(ctx context.Context, txtID string, chat domain.JID, archive bool) error
	ArchiveChatCalls []ChatOperationsArchiveChatCall

	RejectCallFunc  func(ctx context.Context, txtID string, from domain.JID, callID string) error
	RejectCallCalls []ChatOperationsRejectCallCall

	RequestUnavailableMessageFunc  func(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error)
	RequestUnavailableMessageCalls []ChatOperationsRequestUnavailableMessageCall
}

var _ port.ChatOperations = (*ChatOperations)(nil)

// ArchiveChat implementa port.ChatOperations.
func (f *ChatOperations) ArchiveChat(ctx context.Context, txtID string, chat domain.JID, archive bool) error {
	f.ArchiveChatCalls = append(f.ArchiveChatCalls, ChatOperationsArchiveChatCall{Ctx: ctx, TxtID: txtID, Chat: chat, Archive: archive})
	if f.ArchiveChatFunc != nil {
		return f.ArchiveChatFunc(ctx, txtID, chat, archive)
	}
	return nil
}

// RejectCall implementa port.ChatOperations.
func (f *ChatOperations) RejectCall(ctx context.Context, txtID string, from domain.JID, callID string) error {
	f.RejectCallCalls = append(f.RejectCallCalls, ChatOperationsRejectCallCall{Ctx: ctx, TxtID: txtID, From: from, CallID: callID})
	if f.RejectCallFunc != nil {
		return f.RejectCallFunc(ctx, txtID, from, callID)
	}
	return nil
}

// RequestUnavailableMessage implementa port.ChatOperations.
func (f *ChatOperations) RequestUnavailableMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error) {
	f.RequestUnavailableMessageCalls = append(f.RequestUnavailableMessageCalls, ChatOperationsRequestUnavailableMessageCall{Ctx: ctx, TxtID: txtID, Chat: chat, Sender: sender, MessageID: messageID})
	if f.RequestUnavailableMessageFunc != nil {
		return f.RequestUnavailableMessageFunc(ctx, txtID, chat, sender, messageID)
	}
	return domain.UnavailableMessageAck{}, nil
}

// --- NewsletterReader --------------------------------------------------

// NewsletterReaderListSubscribedCall é uma chamada a ListSubscribed.
type NewsletterReaderListSubscribedCall struct {
	Ctx   context.Context
	TxtID string
}

// NewsletterReader é o fake de port.NewsletterReader.
type NewsletterReader struct {
	SessionGuard

	ListSubscribedFunc  func(ctx context.Context, txtID string) (any, error)
	ListSubscribedCalls []NewsletterReaderListSubscribedCall
}

var _ port.NewsletterReader = (*NewsletterReader)(nil)

// ListSubscribed implementa port.NewsletterReader.
func (f *NewsletterReader) ListSubscribed(ctx context.Context, txtID string) (any, error) {
	f.ListSubscribedCalls = append(f.ListSubscribedCalls, NewsletterReaderListSubscribedCall{Ctx: ctx, TxtID: txtID})
	if f.ListSubscribedFunc != nil {
		return f.ListSubscribedFunc(ctx, txtID)
	}
	return nil, nil
}
