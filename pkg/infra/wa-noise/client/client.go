package waclient

import (
	"context"
	"time"

	wanoise "wa-api/internal/wa-noise"
	wapairing "wa-api/internal/wa-noise/capabilities/pairing"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

// Client é a superfície mínima de *wanoise.Client exercitada pelos
// adapters deste pacote. Existe para que os caminhos de erro dos adapters
// (especialmente o ramo ErrNoSession) sejam testáveis sem inicializar um
// cliente real do SDK — o que exigiria conexão com servidores do WhatsApp.
//
// *wanoise.Client satisfaz esta interface por construção: cada método
// abaixo tem a assinatura exata de um método público do SDK. O custo de
// manter a interface é trivial (o compilador acusa um método faltante na
// primeira execução de teste); o benefício é cada adapter poder receber um
// fake que implementa só o que ele chama.
//
// Store() aparece como método (não campo) porque Go proíbe campos em
// interfaces, e o tipo concreto é *store.Device.
type Client interface {
	// Família de presença
	SendPresence(ctx context.Context, state types.Presence) error
	SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error
	SubscribePresence(ctx context.Context, jid types.JID) error

	// Família de mensagens
	MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error
	SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error)
	GenerateMessageID() types.MessageID
	BuildUnavailableMessageRequest(chat, sender types.JID, id string) *waE2E.Message

	// BuildRevoke monta a mensagem de revogacao ("apagar para todos") da
	// mensagem id na conversa chat. sender vazio significa mensagem
	// PROPRIA. BuildEdit monta a substituicao do conteudo da mensagem id
	// por newContent.
	//
	// CAP-10 acrescenta os dois a interface estreita (ADR-001) porque
	// /chat/delete, /chat/delete/message e /chat/send/edit deixaram de so'
	// validar e passaram a mutar de verdade.
	//
	// Nao alarga a FACHADA do fork: internal/wa-noise/main.go ja' exporta
	// `Client = core.Client` (alias de tipo, method set inteiro incluso, e
	// portanto BuildRevoke e BuildEdit). O que se alarga aqui e' o seam
	// local de wa-api.
	BuildRevoke(chat, sender types.JID, id types.MessageID) *waE2E.Message
	BuildEdit(chat types.JID, id types.MessageID, newContent *waE2E.Message) *waE2E.Message

	// BuildPollCreation monta a mensagem de criacao de enquete: o
	// cabecalho, as opcoes em claro e quantas delas podem ser escolhidas.
	// CAP-14 acrescenta este metodo a interface estreita (ADR-001) porque
	// /chat/send/poll deixou de so' validar e passou a criar enquete de
	// verdade.
	//
	// Nao alarga a FACHADA do fork: internal/wa-noise/main.go ja' exporta
	// `Client = core.Client` (alias de tipo, method set inteiro incluso, e
	// portanto BuildPollCreation, definido em
	// internal/wa-noise/core/msgsecret_poll.go:65). O que se alarga aqui
	// e' o seam local de wa-api.
	BuildPollCreation(name string, optionNames []string, selectableOptionCount int) *waE2E.Message

	// Upload sobe um anexo (imagem, video, audio, documento) aos
	// servidores do WhatsApp. CAP-02 acrescenta este metodo a interface
	// estreita (ADR-001) porque o envio de midia real, ao contrario do
	// stub que so' validava, precisa da resposta de upload para montar o
	// protobuf da mensagem.
	Upload(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error)

	// Download baixa e decifra o anexo descrito por uma sub-mensagem
	// protobuf (ImageMessage, VideoMessage, AudioMessage, DocumentMessage
	// ou StickerMessage). CAP-09B acrescenta este metodo a interface
	// estreita (ADR-001) porque as cinco rotas /chat/download* deixaram de
	// so' validar e passaram a baixar de verdade.
	//
	// Nao alarga a FACHADA do fork: internal/wa-noise/main.go ja' exporta
	// `Client = core.Client` (alias de tipo, method set inteiro incluso, e
	// portanto Download) e `DownloadableMessage = core.DownloadableMessage`.
	// O que se alarga aqui e' o seam local de wa-api — a mesma superficie
	// que pkg/infra/media/media.go:73 ja' consumia pelo tipo concreto.
	Download(ctx context.Context, msg wanoise.DownloadableMessage) ([]byte, error)

	// Família de grupos
	GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error)
	GetGroupInfoFromLink(ctx context.Context, code string) (*types.GroupInfo, error)
	GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error)
	GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error)
	CreateGroup(ctx context.Context, req wanoise.ReqCreateGroup) (*types.GroupInfo, error)
	JoinGroupWithLink(ctx context.Context, code string) (types.JID, error)
	LeaveGroup(ctx context.Context, jid types.JID) error
	SetGroupName(ctx context.Context, jid types.JID, name string) error
	SetGroupTopic(ctx context.Context, jid types.JID, previousID, newID, topic string) error
	SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error)
	SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error
	SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error
	SetDisappearingTimer(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error
	UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error)
	GetGroupRequestParticipants(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error)
	UpdateGroupRequestParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantRequestChange) ([]types.GroupParticipant, error)
	SetGroupJoinApprovalMode(ctx context.Context, jid types.JID, mode bool) error

	// Família de contatos/usuários (note que GetLIDForPN/Store.Contacts
	// ficam de fora — o adapter acede-os via Store, não via Client).
	IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error)
	GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error)
	GetProfilePictureInfo(ctx context.Context, jid types.JID, params *wanoise.GetProfilePictureParams) (*types.ProfilePictureInfo, error)

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
	FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error

	// Família de newsletter
	GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error)

	// Família de sessão (controle)

	// PairPhone pede ao servidor do WhatsApp o codigo de pareamento por
	// telefone — a alternativa ao QR. Devolve o codigo que o usuario digita
	// no aparelho. CAP-26 acrescenta este metodo a interface estreita
	// (ADR-001) porque POST /session/pairphone deixou de so' validar e
	// passou a devolver codigo de verdade (F152).
	//
	// Nao alarga a FACHADA do fork: internal/wa-noise/main.go ja' exporta
	// `Client = core.Client` (alias de tipo, method set inteiro incluso, e
	// portanto PairPhone, definido em
	// internal/wa-noise/core/pair-code.go:50). O que se alarga aqui e' o
	// seam local de wa-api.
	//
	// O tipo do clientType e' wapairing.ClientType, e nao um nome da
	// fachada: `PairClientType` da raiz e' um APELIDO de tipo para
	// pairing.ClientType (internal/wa-noise/core/pair-code.go:15), logo os
	// dois sao o MESMO tipo e *wanoise.Client satisfaz esta assinatura sem
	// que main.go precise reexportar nada.
	PairPhone(ctx context.Context, phone string, showPushNotification bool, clientType wapairing.ClientType, clientDisplayName string) (string, error)

	IsConnected() bool
	IsLoggedIn() bool
	Logout(ctx context.Context) error
	Disconnect()

	// Store é exposto como método porque o campo é do tipo concreto
	// *store.Device e Go proíbe campos em interfaces.
	Store() *store.Device
}

// Getter é a função de lookup que os adapters recebem no construtor.
// Em produção é clientManager.GetWaNoiseClient; nos testes é uma função
// controlada pelo caso.
type Getter func(txtID string) Client

// RealClient adapta *wanoise.Client para a interface Client. O método
// Store() existe para uniformizar o campo `Store *store.Device` com os
// demais métodos virtuais (Go proíbe campos em interfaces).
type RealClient struct {
	*wanoise.Client
}

func (r RealClient) Store() *store.Device { return r.Client.Store }

// ClientForGetter converte o getter de produção (devolve *wanoise.Client)
// para o getter da interface. Em produção é a única ponte entre o tipo
// concreto e o seam. Exportado porque pkg/bootstrap é quem o chama.
func ClientForGetter(getConcrete func(txtID string) *wanoise.Client) Getter {
	return func(txtID string) Client {
		c := getConcrete(txtID)
		if c == nil {
			return nil
		}
		return RealClient{c}
	}
}

// Compilação: garante que o tipo concreto satisfaz a interface.
var _ Client = RealClient{}
