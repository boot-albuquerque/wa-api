package testkit

import (
	"context"
	"time"

	"wa-api/internal/noise"
	wamessage "wa-api/internal/noise/capabilities/message"
	wapairing "wa-api/internal/noise/capabilities/pairing"
	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/appstate"
	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// Fake é o fake mínimo de Client. Cada campo é um override
// opcional que o teste pode setar; o default é um valor zero seguro
// (nil para ponteiros, retorno zero para os tipos restantes).
//
// Os métodos do fake DEVEM existir com a assinatura exata da interface.
// Não tentamos cobrir todos os usos aqui — só o que cada adapter chama no
// caminho feliz e no caminho ErrNoSession, que é o alvo desta fase.
type Fake struct {
	SendPresenceFn                   func(ctx context.Context, state types.Presence) error
	SendChatPresenceFn               func(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error
	SubscribePresenceFn              func(ctx context.Context, jid types.JID) error
	MarkReadFn                       func(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error
	SendMessageFn                    func(ctx context.Context, to types.JID, message *waE2E.Message, extra ...noise.SendRequestExtra) (noise.SendResponse, error)
	GenerateMessageIDFn              func() types.MessageID
	BuildUnavailableMessageFn        func(chat, sender types.JID, id string) *waE2E.Message
	BuildRevokeFn                    func(chat, sender types.JID, id types.MessageID) *waE2E.Message
	BuildEditFn                      func(chat types.JID, id types.MessageID, newContent *waE2E.Message) *waE2E.Message
	BuildPollCreationFn              func(name string, optionNames []string, selectableOptionCount int) *waE2E.Message
	BuildPollVoteFn                  func(ctx context.Context, pollInfo *types.MessageInfo, optionNames []string) (*waE2E.Message, error)
	UploadFn                         func(ctx context.Context, plaintext []byte, appInfo noise.MediaType) (noise.UploadResponse, error)
	DownloadFn                       func(ctx context.Context, msg noise.DownloadableMessage) ([]byte, error)
	GetGroupInfoFn                   func(ctx context.Context, jid types.JID) (*types.GroupInfo, error)
	GetGroupInfoFromLinkFn           func(ctx context.Context, code string) (*types.GroupInfo, error)
	GetGroupInviteLinkFn             func(ctx context.Context, jid types.JID, reset bool) (string, error)
	GetJoinedGroupsFn                func(ctx context.Context) ([]*types.GroupInfo, error)
	CreateGroupFn                    func(ctx context.Context, req noise.ReqCreateGroup) (*types.GroupInfo, error)
	JoinGroupWithLinkFn              func(ctx context.Context, code string) (types.JID, error)
	LeaveGroupFn                     func(ctx context.Context, jid types.JID) error
	SetGroupNameFn                   func(ctx context.Context, jid types.JID, name string) error
	SetGroupTopicFn                  func(ctx context.Context, jid types.JID, previousID, newID, topic string) error
	SetGroupPhotoFn                  func(ctx context.Context, jid types.JID, avatar []byte) (string, error)
	SetGroupAnnounceFn               func(ctx context.Context, jid types.JID, announce bool) error
	SetGroupLockedFn                 func(ctx context.Context, jid types.JID, locked bool) error
	SetDisappearingTimerFn           func(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error
	SetDefaultDisappearingTimerFn    func(ctx context.Context, timer time.Duration) error
	UpdateGroupParticipantsFn        func(ctx context.Context, jid types.JID, participantChanges []types.JID, action noise.ParticipantChange) ([]types.GroupParticipant, error)
	GetGroupRequestParticipantsFn    func(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error)
	UpdateGroupRequestParticipantsFn func(ctx context.Context, jid types.JID, participantChanges []types.JID, action noise.ParticipantRequestChange) ([]types.GroupParticipant, error)
	SetGroupJoinApprovalModeFn       func(ctx context.Context, jid types.JID, mode bool) error
	GetSubGroupsFn                   func(ctx context.Context, community types.JID) ([]*types.GroupLinkTarget, error)
	GetLinkedGroupsParticipantsFn    func(ctx context.Context, community types.JID) ([]types.JID, error)
	LinkGroupFn                      func(ctx context.Context, parent, child types.JID) error
	UnlinkGroupFn                    func(ctx context.Context, parent, child types.JID) error
	IsOnWhatsAppFn                   func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)
	GetUserInfoFn                    func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error)
	GetProfilePictureInfoFn          func(ctx context.Context, jid types.JID, params *noise.GetProfilePictureParams) (*types.ProfilePictureInfo, error)
	GetBlocklistFn                   func(ctx context.Context) (*types.Blocklist, error)
	UpdateBlocklistFn                func(ctx context.Context, jid types.JID, pnJID types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error)
	TryFetchPrivacySettingsFn        func(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error)
	SetPrivacySettingFn              func(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error)
	RejectCallFn                     func(ctx context.Context, callFrom types.JID, callID string) error
	SendAppStateFn                   func(ctx context.Context, patch appstate.PatchInfo) error
	FetchAppStateFn                  func(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error
	SetStatusMessageFn               func(ctx context.Context, msg string) error
	BuildHistorySyncRequestFn        func(info *types.MessageInfo, count int) *waE2E.Message
	SendPeerMessageFn                func(ctx context.Context, message *waE2E.Message) (noise.SendResponse, error)
	GetSubscribedNewslettersFn       func(ctx context.Context) ([]*types.NewsletterMetadata, error)

	CreateNewsletterFn               func(ctx context.Context, params noise.CreateNewsletterParams) (*types.NewsletterMetadata, error)
	GetNewsletterInfoFn              func(ctx context.Context, jid types.JID) (*types.NewsletterMetadata, error)
	GetNewsletterInfoWithInviteFn    func(ctx context.Context, key string) (*types.NewsletterMetadata, error)
	FollowNewsletterFn               func(ctx context.Context, jid types.JID) error
	UnfollowNewsletterFn             func(ctx context.Context, jid types.JID) error
	NewsletterToggleMuteFn           func(ctx context.Context, jid types.JID, mute bool) error
	GetNewsletterMessagesFn          func(ctx context.Context, jid types.JID, params *noise.GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error)
	GetNewsletterMessageUpdatesFn    func(ctx context.Context, jid types.JID, params *noise.GetNewsletterUpdatesParams) ([]*types.NewsletterMessage, error)
	NewsletterMarkViewedFn           func(ctx context.Context, jid types.JID, serverIDs []types.MessageServerID) error
	NewsletterSendReactionFn         func(ctx context.Context, jid types.JID, serverID types.MessageServerID, reaction string, messageID types.MessageID) error
	NewsletterSubscribeLiveUpdatesFn func(ctx context.Context, jid types.JID) (time.Duration, error)
	NewsletterDemoteAdminFn          func(ctx context.Context, channelJID, userJID types.JID) error
	NewsletterChangeOwnerFn          func(ctx context.Context, channelJID, newOwnerJID types.JID) error
	NewsletterDeleteFn               func(ctx context.Context, channelJID types.JID) error
	NewsletterCreateAdminInviteFn    func(ctx context.Context, channelJID, userJID types.JID) (noise.NewsletterAdminInvite, error)
	NewsletterAcceptAdminInviteFn    func(ctx context.Context, channelJID types.JID) error
	NewsletterRevokeAdminInviteFn    func(ctx context.Context, channelJID, userJID types.JID) error
	PairPhoneFn                      func(ctx context.Context, phone string, showPushNotification bool, clientType wapairing.ClientType, clientDisplayName string) (string, error)
	IsConnectedFn                    func() bool
	IsLoggedInFn                     func() bool
	LogoutFn                         func(ctx context.Context) error
	DisconnectFn                     func()
	StoreFn                          func() *store.Device
}

func (f *Fake) SendPresence(ctx context.Context, state types.Presence) error {
	if f.SendPresenceFn != nil {
		return f.SendPresenceFn(ctx, state)
	}
	return nil
}

func (f *Fake) SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
	if f.SendChatPresenceFn != nil {
		return f.SendChatPresenceFn(ctx, jid, state, media)
	}
	return nil
}

func (f *Fake) SubscribePresence(ctx context.Context, jid types.JID) error {
	if f.SubscribePresenceFn != nil {
		return f.SubscribePresenceFn(ctx, jid)
	}
	return nil
}

func (f *Fake) MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error {
	if f.MarkReadFn != nil {
		return f.MarkReadFn(ctx, ids, timestamp, chat, sender, receiptTypeExtra...)
	}
	return nil
}

func (f *Fake) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...noise.SendRequestExtra) (noise.SendResponse, error) {
	if f.SendMessageFn != nil {
		return f.SendMessageFn(ctx, to, message, extra...)
	}
	return noise.SendResponse{}, nil
}

func (f *Fake) GenerateMessageID() types.MessageID {
	if f.GenerateMessageIDFn != nil {
		return f.GenerateMessageIDFn()
	}
	return "FAKE-MSG-ID"
}

func (f *Fake) BuildUnavailableMessageRequest(chat, sender types.JID, id string) *waE2E.Message {
	if f.BuildUnavailableMessageFn != nil {
		return f.BuildUnavailableMessageFn(chat, sender, id)
	}
	return &waE2E.Message{}
}

// BuildRevoke monta a MESMA mensagem que o cliente real monta.
//
// ARMADILHA 1 deste repo: dublê mais permissivo que a produção esconde o
// defeito. Este método imita uma REGRA — a de que sender vazio marca a
// revogação como sendo de mensagem PRÓPRIA (FromMe=true, sem Participant)
// e sender de terceiro a marca como de outro — então delega para a regra
// REAL, em internal/wa-noise/capabilities/message/builders.go:39
// (BuildRevoke) e :23 (BuildKey), que é exatamente para onde
// (*core.Client).BuildRevoke delega em
// internal/wa-noise/core/message_builders.go:34.
//
// ownID/ownLID entram vazios porque o fake não tem sessão pareada; o eixo
// que os testes medem é o sender, e com ownID vazio a discriminação de
// BuildKey continua valendo: sender vazio -> FromMe=true; sender não vazio
// e diferente de ownID -> FromMe=false.
func (f *Fake) BuildRevoke(chat, sender types.JID, id types.MessageID) *waE2E.Message {
	if f.BuildRevokeFn != nil {
		return f.BuildRevokeFn(chat, sender, id)
	}
	return wamessage.BuildRevoke(types.EmptyJID, types.EmptyJID, chat, sender, id)
}

// BuildEdit monta a MESMA mensagem que o cliente real monta, delegando para
// internal/wa-noise/capabilities/message/builders.go:101 — para onde
// (*core.Client).BuildEdit delega em
// internal/wa-noise/core/message_builders.go:75. Mesma disciplina de
// BuildRevoke quanto a não inventar uma montagem própria.
func (f *Fake) BuildEdit(chat types.JID, id types.MessageID, newContent *waE2E.Message) *waE2E.Message {
	if f.BuildEditFn != nil {
		return f.BuildEditFn(chat, id, newContent)
	}
	return wamessage.BuildEdit(chat, id, newContent)
}

// BuildPollCreation monta a MESMA mensagem que o cliente real monta,
// delegando para internal/wa-noise/capabilities/message/poll.go:65 — para
// onde (*core.Client).BuildPollCreation delega em
// internal/wa-noise/core/msgsecret_poll.go:65. Mesma disciplina de
// BuildRevoke e BuildEdit quanto a nao inventar uma montagem propria.
//
// ARMADILHA 1 deste repo: duble mais permissivo que a producao esconde o
// defeito. Este metodo imita duas REGRAS que so' a implementacao real tem —
// (a) selectableOptionCount fora de [0, len(optionNames)] e' zerado, e (b)
// MessageContextInfo.MessageSecret e' preenchido com bytes aleatorios, sem o
// qual o voto nao e' decifravel. Uma montagem inventada aqui deixaria os dois
// eixos sem medicao.
func (f *Fake) BuildPollCreation(name string, optionNames []string, selectableOptionCount int) *waE2E.Message {
	if f.BuildPollCreationFn != nil {
		return f.BuildPollCreationFn(name, optionNames, selectableOptionCount)
	}
	return wamessage.BuildPollCreation(name, optionNames, selectableOptionCount)
}

// BuildPollVote delegates to the real implementation by default, same
// discipline as BuildPollCreation. The default produces a message with a
// PollUpdateMessage carrying the encrypted vote hashes — the same path
// (*core.Client).BuildPollVote takes in
// internal/wa-noise/core/msgsecret_poll.go:54, which delegates to
// internal/wa-noise/capabilities/message/poll.go:53.
//
// The fake transport has no Signal session, so the encryption will fail and
// return an error alongside a non-nil *waE2E.Message with PollUpdateMessage
// nil — that is the documented behaviour of BuildPollVote. Tests that need
// the happy path must provide BuildPollVoteFn.
func (f *Fake) BuildPollVote(ctx context.Context, pollInfo *types.MessageInfo, optionNames []string) (*waE2E.Message, error) {
	if f.BuildPollVoteFn != nil {
		return f.BuildPollVoteFn(ctx, pollInfo, optionNames)
	}
	return &waE2E.Message{}, nil
}

func (f *Fake) Upload(ctx context.Context, plaintext []byte, appInfo noise.MediaType) (noise.UploadResponse, error) {
	if f.UploadFn != nil {
		return f.UploadFn(ctx, plaintext, appInfo)
	}
	return noise.UploadResponse{}, nil
}

// Download devolve o que DownloadFn devolver. O default e' (nil, nil) —
// deliberadamente NAO um sucesso com bytes: o caso "sem override" nao deve
// parecer um download bem-sucedido, ou um teste que esquecesse de configurar o
// fake passaria por engano.
func (f *Fake) Download(ctx context.Context, msg noise.DownloadableMessage) ([]byte, error) {
	if f.DownloadFn != nil {
		return f.DownloadFn(ctx, msg)
	}
	return nil, nil
}
