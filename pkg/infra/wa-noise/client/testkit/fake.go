package testkit

import (
	"context"
	"time"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
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
	SendMessageFn                    func(ctx context.Context, to types.JID, message *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error)
	GenerateMessageIDFn              func() types.MessageID
	BuildUnavailableMessageFn        func(chat, sender types.JID, id string) *waE2E.Message
	UploadFn                         func(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error)
	GetGroupInfoFn                   func(ctx context.Context, jid types.JID) (*types.GroupInfo, error)
	GetGroupInfoFromLinkFn           func(ctx context.Context, code string) (*types.GroupInfo, error)
	GetGroupInviteLinkFn             func(ctx context.Context, jid types.JID, reset bool) (string, error)
	GetJoinedGroupsFn                func(ctx context.Context) ([]*types.GroupInfo, error)
	CreateGroupFn                    func(ctx context.Context, req wanoise.ReqCreateGroup) (*types.GroupInfo, error)
	JoinGroupWithLinkFn              func(ctx context.Context, code string) (types.JID, error)
	LeaveGroupFn                     func(ctx context.Context, jid types.JID) error
	SetGroupNameFn                   func(ctx context.Context, jid types.JID, name string) error
	SetGroupTopicFn                  func(ctx context.Context, jid types.JID, previousID, newID, topic string) error
	SetGroupPhotoFn                  func(ctx context.Context, jid types.JID, avatar []byte) (string, error)
	SetGroupAnnounceFn               func(ctx context.Context, jid types.JID, announce bool) error
	SetGroupLockedFn                 func(ctx context.Context, jid types.JID, locked bool) error
	SetDisappearingTimerFn           func(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error
	UpdateGroupParticipantsFn        func(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error)
	GetGroupRequestParticipantsFn    func(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error)
	UpdateGroupRequestParticipantsFn func(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantRequestChange) ([]types.GroupParticipant, error)
	SetGroupJoinApprovalModeFn       func(ctx context.Context, jid types.JID, mode bool) error
	IsOnWhatsAppFn                   func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)
	GetUserInfoFn                    func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error)
	GetProfilePictureInfoFn          func(ctx context.Context, jid types.JID, params *wanoise.GetProfilePictureParams) (*types.ProfilePictureInfo, error)
	GetBlocklistFn                   func(ctx context.Context) (*types.Blocklist, error)
	UpdateBlocklistFn                func(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error)
	TryFetchPrivacySettingsFn        func(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error)
	SetPrivacySettingFn              func(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error)
	RejectCallFn                     func(ctx context.Context, callFrom types.JID, callID string) error
	SendAppStateFn                   func(ctx context.Context, patch appstate.PatchInfo) error
	FetchAppStateFn                  func(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error
	GetSubscribedNewslettersFn       func(ctx context.Context) ([]*types.NewsletterMetadata, error)
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

func (f *Fake) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
	if f.SendMessageFn != nil {
		return f.SendMessageFn(ctx, to, message, extra...)
	}
	return wanoise.SendResponse{}, nil
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

func (f *Fake) Upload(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
	if f.UploadFn != nil {
		return f.UploadFn(ctx, plaintext, appInfo)
	}
	return wanoise.UploadResponse{}, nil
}
