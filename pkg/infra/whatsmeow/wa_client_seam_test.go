package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// fakeWAClient é o fake mínimo de waClient. Cada campo é um override
// opcional que o teste pode setar; o default é um valor zero seguro
// (nil para ponteiros, retorno zero para os tipos restantes).
//
// Os métodos do fake DEVEM existir com a assinatura exata da interface.
// Não tentamos cobrir todos os usos aqui — só o que cada adapter chama no
// caminho feliz e no caminho ErrNoSession, que é o alvo desta fase.
type fakeWAClient struct {
	SendPresenceFn                   func(ctx context.Context, state types.Presence) error
	SendChatPresenceFn               func(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error
	SubscribePresenceFn              func(ctx context.Context, jid types.JID) error
	MarkReadFn                       func(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error
	SendMessageFn                    func(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
	GenerateMessageIDFn              func() types.MessageID
	BuildUnavailableMessageFn        func(chat, sender types.JID, id string) *waE2E.Message
	GetGroupInfoFn                   func(ctx context.Context, jid types.JID) (*types.GroupInfo, error)
	GetGroupInfoFromLinkFn           func(ctx context.Context, code string) (*types.GroupInfo, error)
	GetGroupInviteLinkFn             func(ctx context.Context, jid types.JID, reset bool) (string, error)
	GetJoinedGroupsFn                func(ctx context.Context) ([]*types.GroupInfo, error)
	CreateGroupFn                    func(ctx context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error)
	JoinGroupWithLinkFn              func(ctx context.Context, code string) (types.JID, error)
	LeaveGroupFn                     func(ctx context.Context, jid types.JID) error
	SetGroupNameFn                   func(ctx context.Context, jid types.JID, name string) error
	SetGroupTopicFn                  func(ctx context.Context, jid types.JID, previousID, newID, topic string) error
	SetGroupPhotoFn                  func(ctx context.Context, jid types.JID, avatar []byte) (string, error)
	SetGroupAnnounceFn               func(ctx context.Context, jid types.JID, announce bool) error
	SetGroupLockedFn                 func(ctx context.Context, jid types.JID, locked bool) error
	SetDisappearingTimerFn           func(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error
	UpdateGroupParticipantsFn        func(ctx context.Context, jid types.JID, participantChanges []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error)
	GetGroupRequestParticipantsFn    func(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error)
	UpdateGroupRequestParticipantsFn func(ctx context.Context, jid types.JID, participantChanges []types.JID, action whatsmeow.ParticipantRequestChange) ([]types.GroupParticipant, error)
	SetGroupJoinApprovalModeFn       func(ctx context.Context, jid types.JID, mode bool) error
	IsOnWhatsAppFn                   func(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)
	GetUserInfoFn                    func(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error)
	GetProfilePictureInfoFn          func(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error)
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

func (f *fakeWAClient) SendPresence(ctx context.Context, state types.Presence) error {
	if f.SendPresenceFn != nil {
		return f.SendPresenceFn(ctx, state)
	}
	return nil
}

func (f *fakeWAClient) SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
	if f.SendChatPresenceFn != nil {
		return f.SendChatPresenceFn(ctx, jid, state, media)
	}
	return nil
}

func (f *fakeWAClient) SubscribePresence(ctx context.Context, jid types.JID) error {
	if f.SubscribePresenceFn != nil {
		return f.SubscribePresenceFn(ctx, jid)
	}
	return nil
}

func (f *fakeWAClient) MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error {
	if f.MarkReadFn != nil {
		return f.MarkReadFn(ctx, ids, timestamp, chat, sender, receiptTypeExtra...)
	}
	return nil
}

func (f *fakeWAClient) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	if f.SendMessageFn != nil {
		return f.SendMessageFn(ctx, to, message, extra...)
	}
	return whatsmeow.SendResponse{}, nil
}

func (f *fakeWAClient) GenerateMessageID() types.MessageID {
	if f.GenerateMessageIDFn != nil {
		return f.GenerateMessageIDFn()
	}
	return "FAKE-MSG-ID"
}

func (f *fakeWAClient) BuildUnavailableMessageRequest(chat, sender types.JID, id string) *waE2E.Message {
	if f.BuildUnavailableMessageFn != nil {
		return f.BuildUnavailableMessageFn(chat, sender, id)
	}
	return &waE2E.Message{}
}

func (f *fakeWAClient) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	if f.GetGroupInfoFn != nil {
		return f.GetGroupInfoFn(ctx, jid)
	}
	return nil, nil
}

func (f *fakeWAClient) GetGroupInfoFromLink(ctx context.Context, code string) (*types.GroupInfo, error) {
	if f.GetGroupInfoFromLinkFn != nil {
		return f.GetGroupInfoFromLinkFn(ctx, code)
	}
	return nil, nil
}

func (f *fakeWAClient) GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error) {
	if f.GetGroupInviteLinkFn != nil {
		return f.GetGroupInviteLinkFn(ctx, jid, reset)
	}
	return "", nil
}

func (f *fakeWAClient) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	if f.GetJoinedGroupsFn != nil {
		return f.GetJoinedGroupsFn(ctx)
	}
	return nil, nil
}

func (f *fakeWAClient) CreateGroup(ctx context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error) {
	if f.CreateGroupFn != nil {
		return f.CreateGroupFn(ctx, req)
	}
	return nil, nil
}

func (f *fakeWAClient) JoinGroupWithLink(ctx context.Context, code string) (types.JID, error) {
	if f.JoinGroupWithLinkFn != nil {
		return f.JoinGroupWithLinkFn(ctx, code)
	}
	return types.JID{}, nil
}

func (f *fakeWAClient) LeaveGroup(ctx context.Context, jid types.JID) error {
	if f.LeaveGroupFn != nil {
		return f.LeaveGroupFn(ctx, jid)
	}
	return nil
}

func (f *fakeWAClient) SetGroupName(ctx context.Context, jid types.JID, name string) error {
	if f.SetGroupNameFn != nil {
		return f.SetGroupNameFn(ctx, jid, name)
	}
	return nil
}

func (f *fakeWAClient) SetGroupTopic(ctx context.Context, jid types.JID, previousID, newID, topic string) error {
	if f.SetGroupTopicFn != nil {
		return f.SetGroupTopicFn(ctx, jid, previousID, newID, topic)
	}
	return nil
}

func (f *fakeWAClient) SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
	if f.SetGroupPhotoFn != nil {
		return f.SetGroupPhotoFn(ctx, jid, avatar)
	}
	return "", nil
}

func (f *fakeWAClient) SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error {
	if f.SetGroupAnnounceFn != nil {
		return f.SetGroupAnnounceFn(ctx, jid, announce)
	}
	return nil
}

func (f *fakeWAClient) SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error {
	if f.SetGroupLockedFn != nil {
		return f.SetGroupLockedFn(ctx, jid, locked)
	}
	return nil
}

func (f *fakeWAClient) SetDisappearingTimer(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error {
	if f.SetDisappearingTimerFn != nil {
		return f.SetDisappearingTimerFn(ctx, chat, timer, settingTS)
	}
	return nil
}

func (f *fakeWAClient) UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error) {
	if f.UpdateGroupParticipantsFn != nil {
		return f.UpdateGroupParticipantsFn(ctx, jid, participantChanges, action)
	}
	return nil, nil
}

func (f *fakeWAClient) GetGroupRequestParticipants(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
	if f.GetGroupRequestParticipantsFn != nil {
		return f.GetGroupRequestParticipantsFn(ctx, jid)
	}
	return nil, nil
}

func (f *fakeWAClient) UpdateGroupRequestParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action whatsmeow.ParticipantRequestChange) ([]types.GroupParticipant, error) {
	if f.UpdateGroupRequestParticipantsFn != nil {
		return f.UpdateGroupRequestParticipantsFn(ctx, jid, participantChanges, action)
	}
	return nil, nil
}

func (f *fakeWAClient) SetGroupJoinApprovalMode(ctx context.Context, jid types.JID, mode bool) error {
	if f.SetGroupJoinApprovalModeFn != nil {
		return f.SetGroupJoinApprovalModeFn(ctx, jid, mode)
	}
	return nil
}

func (f *fakeWAClient) IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	if f.IsOnWhatsAppFn != nil {
		return f.IsOnWhatsAppFn(ctx, phones)
	}
	return nil, nil
}

func (f *fakeWAClient) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	if f.GetUserInfoFn != nil {
		return f.GetUserInfoFn(ctx, jids)
	}
	return nil, nil
}

func (f *fakeWAClient) GetProfilePictureInfo(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
	if f.GetProfilePictureInfoFn != nil {
		return f.GetProfilePictureInfoFn(ctx, jid, params)
	}
	return nil, nil
}

func (f *fakeWAClient) GetBlocklist(ctx context.Context) (*types.Blocklist, error) {
	if f.GetBlocklistFn != nil {
		return f.GetBlocklistFn(ctx)
	}
	return nil, nil
}

func (f *fakeWAClient) UpdateBlocklist(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
	if f.UpdateBlocklistFn != nil {
		return f.UpdateBlocklistFn(ctx, jid, action)
	}
	return nil, nil
}

func (f *fakeWAClient) TryFetchPrivacySettings(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error) {
	if f.TryFetchPrivacySettingsFn != nil {
		return f.TryFetchPrivacySettingsFn(ctx, ignoreCache)
	}
	return nil, nil
}

func (f *fakeWAClient) SetPrivacySetting(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error) {
	if f.SetPrivacySettingFn != nil {
		return f.SetPrivacySettingFn(ctx, name, value)
	}
	return types.PrivacySettings{}, nil
}

func (f *fakeWAClient) RejectCall(ctx context.Context, callFrom types.JID, callID string) error {
	if f.RejectCallFn != nil {
		return f.RejectCallFn(ctx, callFrom, callID)
	}
	return nil
}

func (f *fakeWAClient) SendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	if f.SendAppStateFn != nil {
		return f.SendAppStateFn(ctx, patch)
	}
	return nil
}

func (f *fakeWAClient) FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	if f.FetchAppStateFn != nil {
		return f.FetchAppStateFn(ctx, name, fullSync, onlyIfNotSynced)
	}
	return nil
}

func (f *fakeWAClient) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	if f.GetSubscribedNewslettersFn != nil {
		return f.GetSubscribedNewslettersFn(ctx)
	}
	return nil, nil
}

func (f *fakeWAClient) IsConnected() bool {
	if f.IsConnectedFn != nil {
		return f.IsConnectedFn()
	}
	return false
}

func (f *fakeWAClient) IsLoggedIn() bool {
	if f.IsLoggedInFn != nil {
		return f.IsLoggedInFn()
	}
	return false
}

func (f *fakeWAClient) Logout(ctx context.Context) error {
	if f.LogoutFn != nil {
		return f.LogoutFn(ctx)
	}
	return nil
}

func (f *fakeWAClient) Disconnect() {
	if f.DisconnectFn != nil {
		f.DisconnectFn()
	}
}

func (f *fakeWAClient) Store() *store.Device {
	if f.StoreFn != nil {
		return f.StoreFn()
	}
	return nil
}

// getterWith devolve uma waClientGetter que mapeia txtID para o cliente
// correspondente em clients. txtIDs ausentes devolvem nil (que é
// exatamente o comportamento de ClientManager.GetWhatsmeowClient).
func getterWith(clients map[string]waClient) waClientGetter {
	return func(txtID string) waClient {
		return clients[txtID]
	}
}

// errClient é um waClient mínimo que devolve err para uma operação
// específica — usado nos testes de propagação de erro do SDK.
type errClient struct {
	fakeWAClient
	errOp string
	err   error
}

func (e *errClient) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	if e.errOp == "SendMessage" {
		return whatsmeow.SendResponse{}, e.err
	}
	return e.fakeWAClient.SendMessage(ctx, to, message, extra...)
}

// errStore simula um *store.Device com Contacts/LIDs para UserAdapter.
type errStore struct{ *store.Device }

// helper: cria um erro para usar nos testes de propagação.
var testErr = errors.New("synthetic SDK error")

// TestFakeWAClient_SatisfiesInterface é o guarda-compilação: o fake tem
// a forma exata de waClient e waClientGetter; se um dia a interface
// ganhar um método novo, este teste quebra antes de qualquer outro.
func TestFakeWAClient_SatisfiesInterface(t *testing.T) {
	var _ waClient = (*fakeWAClient)(nil)
	var _ waClientGetter = getterWith(nil)
}
