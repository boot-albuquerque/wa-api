package port

import (
	"context"
	"time"

	"wa-api/pkg/domain"
)

// As operações avulsas sobre uma conversa vivem em portas SEPARADAS, uma por
// capacidade (decisão 80).
//
// Antes eram uma interface só. O problema apareceu quando um SEGUNDO transporte
// passou a existir: o adaptador headless dirige a SPA e sabe arquivar, mas
// `RequestUnavailableMessage` não faz sentido nenhum ali — é pedir ao par o
// reenvio de uma mensagem que não pôde ser DECIFRADA, e quem dirige a página
// não decifra nada, a página já entrega texto. Com uma interface única, esse
// adaptador teria de devolver "não suportado" para satisfazer o compilador:
// satisfazer o TIPO enquanto mente sobre a CAPACIDADE.
//
// Portas por capacidade removem a mentira. Quem precisa de arquivar pede
// ChatArchiver, e um transporte que não rejeita chamadas simplesmente não
// implementa CallRejecter — a composição escolhe apenas contratos realmente
// implementados, e a assimetria fica visível no tipo em vez de escondida num
// erro em tempo de execução.

// ChatArchiver arquiva e desarquiva conversas.
type ChatArchiver interface {
	SessionGuard

	// ArchiveChat arquiva ou desarquiva uma conversa.
	ArchiveChat(ctx context.Context, txtID string, chat domain.JID, archive bool) error
}

// ChatMuter mutes and unmutes conversations.
type ChatMuter interface {
	SessionGuard

	// MuteChat mutes or unmutes a conversation. When muting,
	// muteDuration controls for how long (zero = forever).
	MuteChat(ctx context.Context, txtID string, chat domain.JID, mute bool, muteDuration time.Duration) error
}

// ChatPinner pins and unpins conversations in the chat list.
//
// Pin is app-state (like archive), not a message — the response carries no
// message_id. The BuildArchive comment in the fork confirms that archiving a
// chat also unpins it automatically; this port does not enforce that invariant
// because the server does.
type ChatPinner interface {
	SessionGuard

	// PinChat pins or unpins a conversation in the chat list.
	PinChat(ctx context.Context, txtID string, chat domain.JID, pin bool) error
}

// CallRejecter rejeita chamadas recebidas.
type CallRejecter interface {
	SessionGuard

	// RejectCall rejeita uma chamada recebida.
	RejectCall(ctx context.Context, txtID string, from domain.JID, callID string) error
}

// UnavailableMessageRequester pede o reenvio de mensagem indecifrável.
//
// É a mais específica das três por natureza: só faz sentido num transporte que
// FAÇA a decifração, e portanto não é esperado que todo transporte a satisfaça.
type UnavailableMessageRequester interface {
	SessionGuard

	// RequestUnavailableMessage pede ao par o reenvio de uma mensagem que
	// não pôde ser decifrada.
	RequestUnavailableMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error)

	// SetDisappearingTimer sets the disappearing message timer for a
	// specific chat (private or group). CAP-50 adds this alongside the
	// group-only path that already existed in GroupSettings: the SDK
	// method supports both, but the API only exposed the group variant.
	SetDisappearingTimer(ctx context.Context, txtID string, chat domain.JID, d time.Duration, at time.Time) error
}

// DefaultDisappearingTimerSetter sets the account-wide default timer for
// new conversations. Separate from ChatOperations because the target is
// the account, not a chat — no JID involved.
type DefaultDisappearingTimerSetter interface {
	SessionGuard

	// SetDefaultDisappearingTimer changes the default disappearing
	// message timer for all new conversations.
	SetDefaultDisappearingTimer(ctx context.Context, txtID string, d time.Duration) error
}

// ChatOperations é a composição das três, mantida para o adaptador que as
// satisfaz TODAS declarar isso numa linha só. Não é o que um caso de uso deve
// pedir: pedir a composição é voltar a exigir capacidades que não se usa.
type ChatOperations interface {
	ChatArchiver
	ChatMuter
	CallRejecter
	UnavailableMessageRequester
}

// ProfileAccessProvider entrega o ProfileDataAccess da sessão.
//
// Substitui a fábrica func(*wanoise.Client) ProfileDataAccess que
// GetProfileUseCase recebia no construtor: a porta existia, mas o use case
// precisava do cliente concreto do SDK para poder construí-la, o que anulava
// o isolamento que ela deveria dar.
type ProfileAccessProvider interface {
	SessionGuard

	// ProfileAccess devolve o acesso ao perfil da sessão txtID.
	ProfileAccess(ctx context.Context, txtID string) (ProfileDataAccess, error)
}

// NewsletterReader lê as newsletters assinadas pela sessão.
type NewsletterReader interface {
	SessionGuard

	// ListSubscribed devolve as newsletters assinadas.
	ListSubscribed(ctx context.Context, txtID string) ([]domain.NewsletterMetadata, error)

	// As onze abaixo entraram no levantamento de paridade de 2026-08-20: a
	// biblioteca expunha doze capacidades de newsletter e nós expúnhamos UMA.
	//
	// O RETORNO ERA `any` E DEIXOU DE SER. A razão registada era não arrastar
	// `types.NewsletterMetadata` para a aplicação; o efeito medido foi outro: a
	// forma do JSON de `/newsletter/info` passava a ser decidida pelo MOTOR da
	// sessão — a struct do vendor no wa-noise, `channel.DirectoryEntry` (sem
	// etiquetas `json` nenhumas) no headless. Um tipo de domínio próprio resolve
	// os dois problemas; `any` só resolvia o primeiro.
	//
	// Onde não há retorno é porque não há dado a atravessar: Follow, Unfollow,
	// Mute, MarkViewed e React devolvem só sucesso ou erro.

	// CreateNewsletter cria um canal. `picture` é opcional e vai como bytes.
	CreateNewsletter(ctx context.Context, txtID, name, description string, picture []byte) (*domain.NewsletterMetadata, error)

	// NewsletterInfo devolve os metadados de um canal pelo JID.
	NewsletterInfo(ctx context.Context, txtID string, jid domain.JID) (*domain.NewsletterMetadata, error)

	// NewsletterInfoWithInvite devolve os metadados pelo CÓDIGO de convite —
	// que é coisa diferente do JID, e é o que aparece num link partilhado.
	NewsletterInfoWithInvite(ctx context.Context, txtID, inviteKey string) (*domain.NewsletterMetadata, error)

	// FollowNewsletter e UnfollowNewsletter passam a seguir e a deixar de
	// seguir. São separados em vez de um `seguir bool` porque é assim que a
	// biblioteca os expõe, e colapsá-los faria a rota inventar um contrato que
	// o protocolo não tem.
	FollowNewsletter(ctx context.Context, txtID string, jid domain.JID) error
	UnfollowNewsletter(ctx context.Context, txtID string, jid domain.JID) error

	// ToggleNewsletterMute silencia ou deixa de silenciar.
	ToggleNewsletterMute(ctx context.Context, txtID string, jid domain.JID, mute bool) error

	// NewsletterMessages devolve mensagens do canal, paginadas para trás.
	// `count` e `before` a zero deixam o servidor escolher o padrão dele.
	NewsletterMessages(ctx context.Context, txtID string, jid domain.JID, count int, before string) ([]domain.NewsletterMessage, error)

	// NewsletterMessageUpdates devolve ATUALIZAÇÕES de mensagens já vistas —
	// reações e edições —, e não mensagens novas. São consultas diferentes no
	// protocolo e a distinção é do WhatsApp, não nossa.
	NewsletterMessageUpdates(ctx context.Context, txtID string, jid domain.JID, count int, since time.Time, after string) ([]domain.NewsletterMessage, error)

	// MarkNewsletterViewed marca mensagens como vistas, por ID DE SERVIDOR —
	// que é um inteiro do canal, e não o message_id das outras rotas.
	MarkNewsletterViewed(ctx context.Context, txtID string, jid domain.JID, serverIDs []int) error

	// SendNewsletterReaction reage a uma mensagem do canal. `reaction` vazio
	// REMOVE a reação, como no resto do protocolo.
	SendNewsletterReaction(ctx context.Context, txtID string, jid domain.JID, serverID int, reaction, messageID string) error

	// SubscribeNewsletterLiveUpdates liga as atualizações ao vivo e devolve
	// por quanto tempo elas valem — a duração é do servidor, não escolha
	// nossa, e o chamador precisa dela para saber quando repetir.
	SubscribeNewsletterLiveUpdates(ctx context.Context, txtID string, jid domain.JID) (time.Duration, error)

	// F233b — admin management and channel deletion.
	DemoteNewsletterAdmin(ctx context.Context, txtID string, channelJID, userJID domain.JID) error
	ChangeNewsletterOwner(ctx context.Context, txtID string, channelJID, newOwnerJID domain.JID) error
	DeleteNewsletter(ctx context.Context, txtID string, channelJID domain.JID) error

	// F233(b) — admin invite management.
	CreateNewsletterAdminInvite(ctx context.Context, txtID string, channelJID, userJID domain.JID) error
	AcceptNewsletterAdminInvite(ctx context.Context, txtID string, channelJID domain.JID) error
	RevokeNewsletterAdminInvite(ctx context.Context, txtID string, channelJID, userJID domain.JID) error
}

// AppStateSyncer força o pull de um patch de app-state do servidor WhatsApp.
type AppStateSyncer interface {
	SessionGuard

	// SyncContactRoster puxa o patch critical_unblock_low (agenda de
	// contatos). mode: "if_unsynced" (no-op se já sincronizado) |
	// "incremental" (fetch barato, não apaga versão) | "full" (re-snapshot
	// completo, caro).
	SyncContactRoster(ctx context.Context, txtID string, mode string) error
}

// StatusMessageSetter defines the account "about" text.
//
// It exists because `POST /status/set/text` was a REGISTERED ROUTE that
// validated its payload, logged "set status message validated" and answered
// 200 without ever reaching the library — the caller could not tell that from
// success (HOUSEKEEP F198). The port is what gives the use case somewhere to
// go.
type StatusMessageSetter interface {
	SessionGuard
	// SetStatusMessage publishes msg as the account status text.
	SetStatusMessage(ctx context.Context, txtID, msg string) error
}

// HistorySyncRequester asks the primary device for older messages.
//
// Like StatusMessageSetter, it exists because `POST /user/history/sync` was a
// REGISTERED ROUTE that validated and answered 200 without asking for
// anything (HOUSEKEEP F198).
//
// A ASSINATURA CARREGA A ÂNCORA porque o protocolo exige uma: o pedido é
// "os `count` mensagens imediatamente ANTES desta", e sem a mensagem de
// referência não há pedido nenhum a fazer. É por isso que o DTO da rota já
// trazia chatJID, msgID, fromMe e timestamp desde sempre — o contrato HTTP
// estava certo, faltava o caminho.
type HistorySyncRequester interface {
	SessionGuard
	// RequestHistorySync pede `count` mensagens anteriores à mensagem
	// identificada pela âncora. Devolve o id da mensagem de pedido enviada.
	RequestHistorySync(ctx context.Context, txtID string, anchor HistoryAnchor, count int) (string, error)
}

// MessageStarrer stars or unstars a message via app-state patch.
type MessageStarrer interface {
	SessionGuard

	// StarMessage stars or unstars a single message identified by chat,
	// sender, messageID and fromMe.
	StarMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string, fromMe, star bool) error
}

// HistoryAnchor identifica a mensagem a partir da qual se pede para trás.
type HistoryAnchor struct {
	ChatJID   domain.JID
	MessageID string
	FromMe    bool
	Timestamp int64
}
