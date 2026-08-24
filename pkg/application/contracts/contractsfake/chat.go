package contractsfake

import (
	"context"
	"fmt"
	"strings"
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
	// IMITA A TRANSFORMAÇÃO real: JIDResolverAdapter.ResolveJID
	// (mapping/jid/resolver.go:21) aplica o SERVIDOR POR OMISSÃO a um
	// telefone cru — "5511999" sai como "5511999@s.whatsapp.net". O dublê
	// devolvia o texto inalterado, e por isso toda asserção sobre o JID
	// RESOLVIDO descrevia um valor que a produção nunca produz.
	//
	// Armadilha nº1 do ARMADILHAS.md: quando o objeto atravessa uma
	// transformação no caminho real, o dublê tem de atravessá-la também.
	//
	// E imita também a GUARDA, acrescentada pela F209: a produção só aplica o
	// servidor por omissão a algo que PAREÇA um telefone (só dígitos, 5 a 15).
	// `{"Phone":"abc"}` é RECUSADO — sem isso ia para a rede e o pedido ficava
	// pendurado 75 segundos. Um dublê mais permissivo que a produção esconde
	// exatamente o defeito que a guarda existe para travar.
	if !strings.Contains(raw, "@") {
		if !pareceTelefone(raw) {
			return "", fmt.Errorf("contractsfake: %q não é telefone plausível", raw)
		}
		return domain.JID(raw + "@s.whatsapp.net"), nil
	}
	return domain.JID(raw), nil
}

// pareceTelefone espelha `ehTelefonePlausivel` de
// pkg/infra/wa-noise/mapping/jid/parse.go. Os limites vivem lá; se mudarem,
// mudam aqui — é duplicação consciente, porque o dublê não pode importar infra.
func pareceTelefone(raw string) bool {
	if len(raw) < 5 || len(raw) > 15 {
		return false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ResolveQualifiedJID implementa port.JIDResolver.
func (f *JIDResolver) ResolveQualifiedJID(ctx context.Context, raw string) (domain.JID, error) {
	f.ResolveQualifiedJIDCalls = append(f.ResolveQualifiedJIDCalls, JIDResolverResolveQualifiedJIDCall{Ctx: ctx, Raw: raw})
	if f.ResolveQualifiedJIDFunc != nil {
		return f.ResolveQualifiedJIDFunc(ctx, raw)
	}
	// IMITA A REGRA REAL, e não uma mais simples. O
	// JIDResolverAdapter.ResolveQualifiedJID de
	// pkg/infra/wa-noise/mapping/jid/resolver.go:42 RECUSA string sem
	// servidor — "qualificado" no nome existe precisamente para isso, e o
	// comentário de lá explica porquê: o adapter que reparseia usa um
	// ParseJID leniente que aplicaria o servidor padrão, adivinhando a
	// identidade em vez de a exigir.
	//
	// Até 2026-08-21 este dublê devolvia o texto cru e era MAIS PERMISSIVO
	// que a produção. Não escondia um defeito: abençoava código morto. 37
	// sub-testes afirmavam o comportamento de entradas que a produção rejeita
	// com 400 — medido em campo, GET /user/lid/5516981818244 -> 400
	// invalid_jid. Ver HOUSEKEEP F202.
	if !strings.Contains(raw, "@") {
		return "", fmt.Errorf("contractsfake: JID %q has no server (imita resolver.go:42)", raw)
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

// ChatMessengerRevokeMessageCall é uma chamada a RevokeMessage.
type ChatMessengerRevokeMessageCall struct {
	Ctx       context.Context
	TxtID     string
	Target    domain.JID
	MessageID string
}

// ChatMessengerEditMessageCall é uma chamada a EditMessage.
type ChatMessengerEditMessageCall struct {
	Ctx       context.Context
	TxtID     string
	Target    domain.JID
	MessageID string
	NewText   string
	CtxInfo   *domain.EditContextInfo
}

// ChatMessengerSendPollVoteCall é uma chamada a SendPollVote.
type ChatMessengerSendPollVoteCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Payload domain.PollVotePayload
	ID      string
}

// ChatMessenger é o fake de port.ChatMessenger.
type ChatMessenger struct {
	SessionGuard

	MarkReadFunc  func(ctx context.Context, txtID string, ids []string, at time.Time, chat, sender domain.JID) error
	MarkReadCalls []ChatMessengerMarkReadCall

	SendReactionFunc  func(ctx context.Context, txtID string, target domain.JID, reaction domain.Reaction) (domain.MessageSendResult, error)
	SendReactionCalls []ChatMessengerSendReactionCall

	RevokeMessageFunc  func(ctx context.Context, txtID string, target domain.JID, messageID string) (domain.MessageSendResult, error)
	RevokeMessageCalls []ChatMessengerRevokeMessageCall

	EditMessageFunc  func(ctx context.Context, txtID string, target domain.JID, messageID, newText string, ctxInfo *domain.EditContextInfo) (domain.MessageSendResult, error)
	EditMessageCalls []ChatMessengerEditMessageCall

	SendPollVoteFunc  func(ctx context.Context, txtID string, target domain.JID, payload domain.PollVotePayload, id string) (domain.MessageSendResult, error)
	SendPollVoteCalls []ChatMessengerSendPollVoteCall
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

// RevokeMessage implementa port.ChatMessenger.
func (f *ChatMessenger) RevokeMessage(ctx context.Context, txtID string, target domain.JID, messageID string) (domain.MessageSendResult, error) {
	f.RevokeMessageCalls = append(f.RevokeMessageCalls, ChatMessengerRevokeMessageCall{Ctx: ctx, TxtID: txtID, Target: target, MessageID: messageID})
	if f.RevokeMessageFunc != nil {
		return f.RevokeMessageFunc(ctx, txtID, target, messageID)
	}
	return domain.MessageSendResult{}, nil
}

// EditMessage implementa port.ChatMessenger.
func (f *ChatMessenger) EditMessage(ctx context.Context, txtID string, target domain.JID, messageID, newText string, ctxInfo *domain.EditContextInfo) (domain.MessageSendResult, error) {
	f.EditMessageCalls = append(f.EditMessageCalls, ChatMessengerEditMessageCall{Ctx: ctx, TxtID: txtID, Target: target, MessageID: messageID, NewText: newText, CtxInfo: ctxInfo})
	if f.EditMessageFunc != nil {
		return f.EditMessageFunc(ctx, txtID, target, messageID, newText, ctxInfo)
	}
	return domain.MessageSendResult{}, nil
}

// SendPollVote implementa port.ChatMessenger.
func (f *ChatMessenger) SendPollVote(ctx context.Context, txtID string, target domain.JID, payload domain.PollVotePayload, id string) (domain.MessageSendResult, error) {
	f.SendPollVoteCalls = append(f.SendPollVoteCalls, ChatMessengerSendPollVoteCall{Ctx: ctx, TxtID: txtID, Target: target, Payload: payload, ID: id})
	if f.SendPollVoteFunc != nil {
		return f.SendPollVoteFunc(ctx, txtID, target, payload, id)
	}
	return domain.MessageSendResult{}, nil
}

// --- ChatMuter --------------------------------------------------------

// ChatMuterMuteChatCall records a call to MuteChat.
type ChatMuterMuteChatCall struct {
	Ctx          context.Context
	TxtID        string
	Chat         domain.JID
	Mute         bool
	MuteDuration time.Duration
}

// ChatMuter is the fake for port.ChatMuter.
type ChatMuter struct {
	SessionGuard

	MuteChatFunc  func(ctx context.Context, txtID string, chat domain.JID, mute bool, muteDuration time.Duration) error
	MuteChatCalls []ChatMuterMuteChatCall
}

var _ port.ChatMuter = (*ChatMuter)(nil)

// MuteChat implements port.ChatMuter.
func (f *ChatMuter) MuteChat(ctx context.Context, txtID string, chat domain.JID, mute bool, muteDuration time.Duration) error {
	f.MuteChatCalls = append(f.MuteChatCalls, ChatMuterMuteChatCall{Ctx: ctx, TxtID: txtID, Chat: chat, Mute: mute, MuteDuration: muteDuration})
	if f.MuteChatFunc != nil {
		return f.MuteChatFunc(ctx, txtID, chat, mute, muteDuration)
	}
	return nil
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

// ChatOperationsSetDisappearingTimerCall é uma chamada a SetDisappearingTimer.
type ChatOperationsSetDisappearingTimerCall struct {
	Ctx      context.Context
	TxtID    string
	Chat     domain.JID
	Duration time.Duration
	At       time.Time
}

// ChatOperations é o fake de port.ChatOperations.
type ChatOperations struct {
	SessionGuard

	ArchiveChatFunc  func(ctx context.Context, txtID string, chat domain.JID, archive bool) error
	ArchiveChatCalls []ChatOperationsArchiveChatCall

	MuteChatFunc  func(ctx context.Context, txtID string, chat domain.JID, mute bool, muteDuration time.Duration) error
	MuteChatCalls []ChatMuterMuteChatCall

	RejectCallFunc  func(ctx context.Context, txtID string, from domain.JID, callID string) error
	RejectCallCalls []ChatOperationsRejectCallCall

	RequestUnavailableMessageFunc  func(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error)
	RequestUnavailableMessageCalls []ChatOperationsRequestUnavailableMessageCall

	SetDisappearingTimerFunc  func(ctx context.Context, txtID string, chat domain.JID, d time.Duration, at time.Time) error
	SetDisappearingTimerCalls []ChatOperationsSetDisappearingTimerCall
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

// MuteChat implements port.ChatOperations.
func (f *ChatOperations) MuteChat(ctx context.Context, txtID string, chat domain.JID, mute bool, muteDuration time.Duration) error {
	f.MuteChatCalls = append(f.MuteChatCalls, ChatMuterMuteChatCall{Ctx: ctx, TxtID: txtID, Chat: chat, Mute: mute, MuteDuration: muteDuration})
	if f.MuteChatFunc != nil {
		return f.MuteChatFunc(ctx, txtID, chat, mute, muteDuration)
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

// SetDisappearingTimer implementa port.ChatOperations.
func (f *ChatOperations) SetDisappearingTimer(ctx context.Context, txtID string, chat domain.JID, d time.Duration, at time.Time) error {
	f.SetDisappearingTimerCalls = append(f.SetDisappearingTimerCalls, ChatOperationsSetDisappearingTimerCall{Ctx: ctx, TxtID: txtID, Chat: chat, Duration: d, At: at})
	if f.SetDisappearingTimerFunc != nil {
		return f.SetDisappearingTimerFunc(ctx, txtID, chat, d, at)
	}
	return nil
}

// --- ChatPinner -------------------------------------------------------

// ChatPinnerPinChatCall is a call to PinChat.
type ChatPinnerPinChatCall struct {
	Ctx   context.Context
	TxtID string
	Chat  domain.JID
	Pin   bool
}

// ChatPinner is the fake for port.ChatPinner.
type ChatPinner struct {
	SessionGuard

	PinChatFunc  func(ctx context.Context, txtID string, chat domain.JID, pin bool) error
	PinChatCalls []ChatPinnerPinChatCall
}

var _ port.ChatPinner = (*ChatPinner)(nil)

// PinChat implements port.ChatPinner.
func (f *ChatPinner) PinChat(ctx context.Context, txtID string, chat domain.JID, pin bool) error {
	f.PinChatCalls = append(f.PinChatCalls, ChatPinnerPinChatCall{Ctx: ctx, TxtID: txtID, Chat: chat, Pin: pin})
	if f.PinChatFunc != nil {
		return f.PinChatFunc(ctx, txtID, chat, pin)
	}
	return nil
}

// --- DefaultDisappearingTimerSetter ------------------------------------

// DefaultDisappearingTimerSetterCall é uma chamada a
// SetDefaultDisappearingTimer.
type DefaultDisappearingTimerSetterCall struct {
	Ctx      context.Context
	TxtID    string
	Duration time.Duration
}

// DefaultDisappearingTimerSetter é o fake de
// port.DefaultDisappearingTimerSetter.
type DefaultDisappearingTimerSetter struct {
	SessionGuard

	SetDefaultDisappearingTimerFunc  func(ctx context.Context, txtID string, d time.Duration) error
	SetDefaultDisappearingTimerCalls []DefaultDisappearingTimerSetterCall
}

var _ port.DefaultDisappearingTimerSetter = (*DefaultDisappearingTimerSetter)(nil)

// SetDefaultDisappearingTimer implementa
// port.DefaultDisappearingTimerSetter.
func (f *DefaultDisappearingTimerSetter) SetDefaultDisappearingTimer(ctx context.Context, txtID string, d time.Duration) error {
	f.SetDefaultDisappearingTimerCalls = append(f.SetDefaultDisappearingTimerCalls, DefaultDisappearingTimerSetterCall{Ctx: ctx, TxtID: txtID, Duration: d})
	if f.SetDefaultDisappearingTimerFunc != nil {
		return f.SetDefaultDisappearingTimerFunc(ctx, txtID, d)
	}
	return nil
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

	// As onze abaixo entraram com o levantamento de paridade de 2026-08-20.
	//
	// Cada uma guarda o JID recebido, e não só a contagem de chamadas: o modo
	// de falha mais provável destas rotas é passar o identificador errado — o
	// código de convite onde ia o JID, ou o JID do canal onde ia o da
	// conversa — e uma contagem não distingue isso de sucesso.
	CreateNewsletterFunc func(ctx context.Context, txtID, name, description string, picture []byte) (any, error)
	NewsletterInfoFunc   func(ctx context.Context, txtID string, jid domain.JID) (any, error)
	NewsletterInviteFunc func(ctx context.Context, txtID, inviteKey string) (any, error)
	FollowFunc           func(ctx context.Context, txtID string, jid domain.JID) error
	UnfollowFunc         func(ctx context.Context, txtID string, jid domain.JID) error
	MuteFunc             func(ctx context.Context, txtID string, jid domain.JID, mute bool) error
	MessagesFunc         func(ctx context.Context, txtID string, jid domain.JID, count int, before string) (any, error)
	UpdatesFunc          func(ctx context.Context, txtID string, jid domain.JID, count int, since time.Time, after string) (any, error)
	MarkViewedFunc       func(ctx context.Context, txtID string, jid domain.JID, serverIDs []int) error
	ReactFunc            func(ctx context.Context, txtID string, jid domain.JID, serverID int, reaction, messageID string) error
	SubscribeLiveFunc    func(ctx context.Context, txtID string, jid domain.JID) (time.Duration, error)

	// NewsletterCalls regista TODA chamada da família, com o método e o
	// identificador. Uma lista só serve para asserir que a rota certa chamou o
	// método certo com o argumento certo — que é o que os testes precisam.
	NewsletterCalls []NewsletterCall
}

// NewsletterCall é uma chamada da família de newsletter.
type NewsletterCall struct {
	Method string
	TxtID  string
	JID    domain.JID
	Extra  string
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

// --- AppStateSyncer ------------------------------------------------------

// AppStateSyncerSyncContactRosterCall é uma chamada a SyncContactRoster.
type AppStateSyncerSyncContactRosterCall struct {
	Ctx   context.Context
	TxtID string
	Mode  string
}

// AppStateSyncer é o fake de port.AppStateSyncer.
type AppStateSyncer struct {
	SessionGuard

	SyncContactRosterFunc  func(ctx context.Context, txtID string, mode string) error
	SyncContactRosterCalls []AppStateSyncerSyncContactRosterCall
}

var _ port.AppStateSyncer = (*AppStateSyncer)(nil)

// SyncContactRoster implementa port.AppStateSyncer.
func (f *AppStateSyncer) SyncContactRoster(ctx context.Context, txtID string, mode string) error {
	f.SyncContactRosterCalls = append(f.SyncContactRosterCalls, AppStateSyncerSyncContactRosterCall{Ctx: ctx, TxtID: txtID, Mode: mode})
	if f.SyncContactRosterFunc != nil {
		return f.SyncContactRosterFunc(ctx, txtID, mode)
	}
	return nil
}

// --- família de newsletter (paridade 2026-08-20) ------------------------------

func (f *NewsletterReader) record(method, txtID string, jid domain.JID, extra string) {
	f.NewsletterCalls = append(f.NewsletterCalls, NewsletterCall{Method: method, TxtID: txtID, JID: jid, Extra: extra})
}

// CreateNewsletter implementa port.NewsletterReader.
func (f *NewsletterReader) CreateNewsletter(ctx context.Context, txtID, name, description string, picture []byte) (any, error) {
	f.record("CreateNewsletter", txtID, "", name)
	if f.CreateNewsletterFunc != nil {
		return f.CreateNewsletterFunc(ctx, txtID, name, description, picture)
	}
	return nil, nil
}

// NewsletterInfo implementa port.NewsletterReader.
func (f *NewsletterReader) NewsletterInfo(ctx context.Context, txtID string, jid domain.JID) (any, error) {
	f.record("NewsletterInfo", txtID, jid, "")
	if f.NewsletterInfoFunc != nil {
		return f.NewsletterInfoFunc(ctx, txtID, jid)
	}
	return nil, nil
}

// NewsletterInfoWithInvite implementa port.NewsletterReader.
func (f *NewsletterReader) NewsletterInfoWithInvite(ctx context.Context, txtID, inviteKey string) (any, error) {
	// O `inviteKey` vai em Extra e NÃO em JID de propósito: é um código de
	// convite, não um identificador de conversa. Guardá-lo no campo de JID
	// faria um teste de "passou o identificador certo" passar com os dois
	// trocados.
	f.record("NewsletterInfoWithInvite", txtID, "", inviteKey)
	if f.NewsletterInviteFunc != nil {
		return f.NewsletterInviteFunc(ctx, txtID, inviteKey)
	}
	return nil, nil
}

// FollowNewsletter implementa port.NewsletterReader.
func (f *NewsletterReader) FollowNewsletter(ctx context.Context, txtID string, jid domain.JID) error {
	f.record("FollowNewsletter", txtID, jid, "")
	if f.FollowFunc != nil {
		return f.FollowFunc(ctx, txtID, jid)
	}
	return nil
}

// UnfollowNewsletter implementa port.NewsletterReader.
func (f *NewsletterReader) UnfollowNewsletter(ctx context.Context, txtID string, jid domain.JID) error {
	f.record("UnfollowNewsletter", txtID, jid, "")
	if f.UnfollowFunc != nil {
		return f.UnfollowFunc(ctx, txtID, jid)
	}
	return nil
}

// ToggleNewsletterMute implementa port.NewsletterReader.
func (f *NewsletterReader) ToggleNewsletterMute(ctx context.Context, txtID string, jid domain.JID, mute bool) error {
	f.record("ToggleNewsletterMute", txtID, jid, fmt.Sprintf("%t", mute))
	if f.MuteFunc != nil {
		return f.MuteFunc(ctx, txtID, jid, mute)
	}
	return nil
}

// NewsletterMessages implementa port.NewsletterReader.
func (f *NewsletterReader) NewsletterMessages(ctx context.Context, txtID string, jid domain.JID, count int, before string) (any, error) {
	f.record("NewsletterMessages", txtID, jid, before)
	if f.MessagesFunc != nil {
		return f.MessagesFunc(ctx, txtID, jid, count, before)
	}
	return nil, nil
}

// NewsletterMessageUpdates implementa port.NewsletterReader.
func (f *NewsletterReader) NewsletterMessageUpdates(ctx context.Context, txtID string, jid domain.JID, count int, since time.Time, after string) (any, error) {
	f.record("NewsletterMessageUpdates", txtID, jid, after)
	if f.UpdatesFunc != nil {
		return f.UpdatesFunc(ctx, txtID, jid, count, since, after)
	}
	return nil, nil
}

// MarkNewsletterViewed implementa port.NewsletterReader.
func (f *NewsletterReader) MarkNewsletterViewed(ctx context.Context, txtID string, jid domain.JID, serverIDs []int) error {
	f.record("MarkNewsletterViewed", txtID, jid, fmt.Sprintf("%v", serverIDs))
	if f.MarkViewedFunc != nil {
		return f.MarkViewedFunc(ctx, txtID, jid, serverIDs)
	}
	return nil
}

// SendNewsletterReaction implementa port.NewsletterReader.
func (f *NewsletterReader) SendNewsletterReaction(ctx context.Context, txtID string, jid domain.JID, serverID int, reaction, messageID string) error {
	f.record("SendNewsletterReaction", txtID, jid, reaction)
	if f.ReactFunc != nil {
		return f.ReactFunc(ctx, txtID, jid, serverID, reaction, messageID)
	}
	return nil
}

// SubscribeNewsletterLiveUpdates implementa port.NewsletterReader.
func (f *NewsletterReader) SubscribeNewsletterLiveUpdates(ctx context.Context, txtID string, jid domain.JID) (time.Duration, error) {
	f.record("SubscribeNewsletterLiveUpdates", txtID, jid, "")
	if f.SubscribeLiveFunc != nil {
		return f.SubscribeLiveFunc(ctx, txtID, jid)
	}
	return 0, nil
}

// --- MessageStarrer -------------------------------------------------------

// MessageStarrerStarMessageCall records a call to StarMessage.
type MessageStarrerStarMessageCall struct {
	Ctx       context.Context
	TxtID     string
	Chat      domain.JID
	Sender    domain.JID
	MessageID string
	FromMe    bool
	Star      bool
}

// MessageStarrer is the fake of port.MessageStarrer.
type MessageStarrer struct {
	SessionGuard

	StarMessageFunc  func(ctx context.Context, txtID string, chat, sender domain.JID, messageID string, fromMe, star bool) error
	StarMessageCalls []MessageStarrerStarMessageCall
}

var _ port.MessageStarrer = (*MessageStarrer)(nil)

// StarMessage implements port.MessageStarrer.
func (f *MessageStarrer) StarMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string, fromMe, star bool) error {
	f.StarMessageCalls = append(f.StarMessageCalls, MessageStarrerStarMessageCall{
		Ctx: ctx, TxtID: txtID, Chat: chat, Sender: sender,
		MessageID: messageID, FromMe: fromMe, Star: star,
	})
	if f.StarMessageFunc != nil {
		return f.StarMessageFunc(ctx, txtID, chat, sender, messageID, fromMe, star)
	}
	return nil
}
