package whatsmeow

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// waClient é a superfície mínima de *whatsmeow.Client exercitada pelos
// adapters deste pacote. Existe para que os caminhos de erro dos adapters
// (especialmente o ramo ErrNoSession) sejam testáveis sem inicializar um
// cliente real do SDK — o que exigiria conexão com servidores do WhatsApp.
//
// *whatsmeow.Client satisfaz esta interface por construção: cada método
// abaixo tem a assinatura exata de um método público do SDK. O custo de
// manter a interface é trivial (o compilador acusa um método faltante na
// primeira execução de teste); o benefício é cada adapter poder receber um
// fake que implementa só o que ele chama.
//
// Store() aparece como método (não campo) porque Go proíbe campos em
// interfaces, e o tipo concreto é *store.Device.
type waClient interface {
	// Família de presença
	SendPresence(ctx context.Context, state types.Presence) error
	SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error
	SubscribePresence(ctx context.Context, jid types.JID) error

	// Família de mensagens
	MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error
	SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
	GenerateMessageID() types.MessageID
	BuildUnavailableMessageRequest(chat, sender types.JID, id string) *waE2E.Message

	// Família de grupos
	GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error)
	GetGroupInfoFromLink(ctx context.Context, code string) (*types.GroupInfo, error)
	GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error)
	GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error)
	CreateGroup(ctx context.Context, req whatsmeow.ReqCreateGroup) (*types.GroupInfo, error)
	JoinGroupWithLink(ctx context.Context, code string) (types.JID, error)
	LeaveGroup(ctx context.Context, jid types.JID) error
	SetGroupName(ctx context.Context, jid types.JID, name string) error
	SetGroupTopic(ctx context.Context, jid types.JID, previousID, newID, topic string) error
	SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error)
	SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error
	SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error
	SetDisappearingTimer(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error
	UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action whatsmeow.ParticipantChange) ([]types.GroupParticipant, error)
	GetGroupRequestParticipants(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error)
	UpdateGroupRequestParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action whatsmeow.ParticipantRequestChange) ([]types.GroupParticipant, error)
	SetGroupJoinApprovalMode(ctx context.Context, jid types.JID, mode bool) error

	// Família de contatos/usuários (note que GetLIDForPN/Store.Contacts
	// ficam de fora — o adapter acede-os via Store, não via Client).
	IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)
	GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error)
	GetProfilePictureInfo(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error)

	// Família de blocklist
	GetBlocklist(ctx context.Context) (*types.Blocklist, error)
	UpdateBlocklist(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error)

	// Família de privacidade
	TryFetchPrivacySettings(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error)
	SetPrivacySetting(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error)

	// Família de chamadas
	RejectCall(ctx context.Context, callFrom types.JID, callID string) error

	// Família de app state
	SendAppState(ctx context.Context, patch appstate.PatchInfo) error

	// Família de newsletter
	GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error)

	// Família de sessão (controle)
	IsConnected() bool
	IsLoggedIn() bool
	Logout(ctx context.Context) error
	Disconnect()

	// Store é exposto como método porque o campo é do tipo concreto
	// *store.Device e Go proíbe campos em interfaces.
	Store() *store.Device
}

// waClientGetter é a função de lookup que os adapters recebem no construtor.
// Em produção é clientManager.GetWhatsmeowClient; nos testes é uma função
// controlada pelo caso.
type waClientGetter func(txtID string) waClient

// realWAClient adapta *whatsmeow.Client para a interface waClient. O método
// Store() existe para uniformizar o campo `Store *store.Device` com os
// demais métodos virtuais (Go proíbe campos em interfaces).
type realWAClient struct {
	*whatsmeow.Client
}

func (r realWAClient) Store() *store.Device { return r.Client.Store }

// ClientForGetter converte o getter de produção (devolve *whatsmeow.Client)
// para o getter da interface. Em produção é a única ponte entre o tipo
// concreto e o seam. Exportado porque pkg/bootstrap é quem o chama.
func ClientForGetter(getConcrete func(txtID string) *whatsmeow.Client) waClientGetter {
	return func(txtID string) waClient {
		c := getConcrete(txtID)
		if c == nil {
			return nil
		}
		return realWAClient{c}
	}
}

// Compilação: garante que o tipo concreto satisfaz a interface.
var _ waClient = realWAClient{}