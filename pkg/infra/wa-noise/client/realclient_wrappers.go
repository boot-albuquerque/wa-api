// Wrappers do RealClient que traduzem recusas do servidor do WhatsApp.
//
// Existem porque a F204 mediu um `400 bad-request` de montante a chegar ao
// cliente como `500 internal server error`, e porque não havia ponto único por
// onde corrigir isso: os adaptadores devolvem o erro cru do SDK, e `RealClient`
// PROMOVIA os métodos de `*wanoise.Client` em vez de os escrever — promoção não
// tem onde se intercalar.
//
// Decisão 46=a do canal, contra as duas alternativas: chamar `ClassifyIQ` em
// cada adaptador (~63 sítios, e o próximo adaptador nasce sem a chamada) e
// classificar no `RespondJSON` (ponto único, mas obriga a presentation a
// conhecer o vocabulário do fork).
//
// O CUSTO, que é real e está aqui escrito porque não se paga sozinho: o
// comentário de `client.go:21` defende que cada método da interface tenha "a
// assinatura exata de um método público do SDK", e que o compilador acuse um
// método em falta na primeira execução de teste. Isso continua verdade — a
// asserção de compilação no fim de `client.go` garante-o —, mas agora um método
// NOVO do SDK que entre na interface passa a precisar também de um wrapper
// aqui, e ninguém o vai lembrar. Enquanto não houver gate a exigi-lo, o
// esquecimento manifesta-se como um 500 a voltar para uma rota só.
//
// `ClassifyIQ` deixa passar intacto tudo o que não seja recusa de info query,
// incluindo nil, por isso envolver métodos que nunca fazem info query é inócuo
// — e é preferível a manter uma lista de quais fazem, que envelheceria pior.
//
// Gerado a partir da interface `Client`; não editar à mão.

package waclient

import (
	"context"
	"time"

	wanoise "wa-api/internal/wa-noise"
	wapairing "wa-api/internal/wa-noise/capabilities/pairing"
	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
	"wa-api/pkg/infra/wa-noise/errmap"
)

func (r RealClient) SendPresence(ctx context.Context, state types.Presence) error {
	return errmap.ClassifyIQ(r.Client.SendPresence(ctx, state))
}

func (r RealClient) SendChatPresence(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
	return errmap.ClassifyIQ(r.Client.SendChatPresence(ctx, jid, state, media))
}

func (r RealClient) SubscribePresence(ctx context.Context, jid types.JID) error {
	return errmap.ClassifyIQ(r.Client.SubscribePresence(ctx, jid))
}

func (r RealClient) MarkRead(ctx context.Context, ids []types.MessageID, timestamp time.Time, chat, sender types.JID, receiptTypeExtra ...types.ReceiptType) error {
	return errmap.ClassifyIQ(r.Client.MarkRead(ctx, ids, timestamp, chat, sender, receiptTypeExtra...))
}

func (r RealClient) SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
	v0, err := r.Client.SendMessage(ctx, to, message, extra...)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) BuildPollVote(ctx context.Context, pollInfo *types.MessageInfo, optionNames []string) (*waE2E.Message, error) {
	v0, err := r.Client.BuildPollVote(ctx, pollInfo, optionNames)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) Upload(ctx context.Context, plaintext []byte, appInfo wanoise.MediaType) (wanoise.UploadResponse, error) {
	v0, err := r.Client.Upload(ctx, plaintext, appInfo)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) Download(ctx context.Context, msg wanoise.DownloadableMessage) ([]byte, error) {
	v0, err := r.Client.Download(ctx, msg)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetGroupInfo(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	v0, err := r.Client.GetGroupInfo(ctx, jid)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetGroupInfoFromLink(ctx context.Context, code string) (*types.GroupInfo, error) {
	v0, err := r.Client.GetGroupInfoFromLink(ctx, code)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetGroupInviteLink(ctx context.Context, jid types.JID, reset bool) (string, error) {
	v0, err := r.Client.GetGroupInviteLink(ctx, jid, reset)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	v0, err := r.Client.GetJoinedGroups(ctx)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) CreateGroup(ctx context.Context, req wanoise.ReqCreateGroup) (*types.GroupInfo, error) {
	v0, err := r.Client.CreateGroup(ctx, req)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) JoinGroupWithLink(ctx context.Context, code string) (types.JID, error) {
	v0, err := r.Client.JoinGroupWithLink(ctx, code)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) LeaveGroup(ctx context.Context, jid types.JID) error {
	return errmap.ClassifyIQ(r.Client.LeaveGroup(ctx, jid))
}

func (r RealClient) SetGroupName(ctx context.Context, jid types.JID, name string) error {
	return errmap.ClassifyIQ(r.Client.SetGroupName(ctx, jid, name))
}

func (r RealClient) SetGroupTopic(ctx context.Context, jid types.JID, previousID, newID, topic string) error {
	return errmap.ClassifyIQ(r.Client.SetGroupTopic(ctx, jid, previousID, newID, topic))
}

func (r RealClient) SetGroupPhoto(ctx context.Context, jid types.JID, avatar []byte) (string, error) {
	v0, err := r.Client.SetGroupPhoto(ctx, jid, avatar)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) SetGroupAnnounce(ctx context.Context, jid types.JID, announce bool) error {
	return errmap.ClassifyIQ(r.Client.SetGroupAnnounce(ctx, jid, announce))
}

func (r RealClient) SetGroupLocked(ctx context.Context, jid types.JID, locked bool) error {
	return errmap.ClassifyIQ(r.Client.SetGroupLocked(ctx, jid, locked))
}

func (r RealClient) SetDisappearingTimer(ctx context.Context, chat types.JID, timer time.Duration, settingTS time.Time) error {
	return errmap.ClassifyIQ(r.Client.SetDisappearingTimer(ctx, chat, timer, settingTS))
}

func (r RealClient) SetDefaultDisappearingTimer(ctx context.Context, timer time.Duration) error {
	return errmap.ClassifyIQ(r.Client.SetDefaultDisappearingTimer(ctx, timer))
}

func (r RealClient) UpdateGroupParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantChange) ([]types.GroupParticipant, error) {
	v0, err := r.Client.UpdateGroupParticipants(ctx, jid, participantChanges, action)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetGroupRequestParticipants(ctx context.Context, jid types.JID) ([]types.GroupParticipantRequest, error) {
	v0, err := r.Client.GetGroupRequestParticipants(ctx, jid)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) UpdateGroupRequestParticipants(ctx context.Context, jid types.JID, participantChanges []types.JID, action wanoise.ParticipantRequestChange) ([]types.GroupParticipant, error) {
	v0, err := r.Client.UpdateGroupRequestParticipants(ctx, jid, participantChanges, action)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) SetGroupJoinApprovalMode(ctx context.Context, jid types.JID, mode bool) error {
	return errmap.ClassifyIQ(r.Client.SetGroupJoinApprovalMode(ctx, jid, mode))
}

func (r RealClient) IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	v0, err := r.Client.IsOnWhatsApp(ctx, phones)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	v0, err := r.Client.GetUserInfo(ctx, jids)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetProfilePictureInfo(ctx context.Context, jid types.JID, params *wanoise.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
	v0, err := r.Client.GetProfilePictureInfo(ctx, jid, params)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetBlocklist(ctx context.Context) (*types.Blocklist, error) {
	v0, err := r.Client.GetBlocklist(ctx)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) UpdateBlocklist(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
	v0, err := r.Client.UpdateBlocklist(ctx, jid, action)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) SetStatusMessage(ctx context.Context, msg string) error {
	return errmap.ClassifyIQ(r.Client.SetStatusMessage(ctx, msg))
}

func (r RealClient) SendPeerMessage(ctx context.Context, message *waE2E.Message) (wanoise.SendResponse, error) {
	v0, err := r.Client.SendPeerMessage(ctx, message)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) TryFetchPrivacySettings(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error) {
	v0, err := r.Client.TryFetchPrivacySettings(ctx, ignoreCache)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) SetPrivacySetting(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error) {
	v0, err := r.Client.SetPrivacySetting(ctx, name, value)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) RejectCall(ctx context.Context, callFrom types.JID, callID string) error {
	return errmap.ClassifyIQ(r.Client.RejectCall(ctx, callFrom, callID))
}

func (r RealClient) SendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	return errmap.ClassifyIQ(r.Client.SendAppState(ctx, patch))
}

func (r RealClient) FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	return errmap.ClassifyIQ(r.Client.FetchAppState(ctx, name, fullSync, onlyIfNotSynced))
}

func (r RealClient) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	v0, err := r.Client.GetSubscribedNewsletters(ctx)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) CreateNewsletter(ctx context.Context, params wanoise.CreateNewsletterParams) (*types.NewsletterMetadata, error) {
	v0, err := r.Client.CreateNewsletter(ctx, params)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetNewsletterInfo(ctx context.Context, jid types.JID) (*types.NewsletterMetadata, error) {
	v0, err := r.Client.GetNewsletterInfo(ctx, jid)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetNewsletterInfoWithInvite(ctx context.Context, key string) (*types.NewsletterMetadata, error) {
	v0, err := r.Client.GetNewsletterInfoWithInvite(ctx, key)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) FollowNewsletter(ctx context.Context, jid types.JID) error {
	return errmap.ClassifyIQ(r.Client.FollowNewsletter(ctx, jid))
}

func (r RealClient) UnfollowNewsletter(ctx context.Context, jid types.JID) error {
	return errmap.ClassifyIQ(r.Client.UnfollowNewsletter(ctx, jid))
}

func (r RealClient) NewsletterToggleMute(ctx context.Context, jid types.JID, mute bool) error {
	return errmap.ClassifyIQ(r.Client.NewsletterToggleMute(ctx, jid, mute))
}

func (r RealClient) GetNewsletterMessages(ctx context.Context, jid types.JID, params *wanoise.GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error) {
	v0, err := r.Client.GetNewsletterMessages(ctx, jid, params)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) GetNewsletterMessageUpdates(ctx context.Context, jid types.JID, params *wanoise.GetNewsletterUpdatesParams) ([]*types.NewsletterMessage, error) {
	v0, err := r.Client.GetNewsletterMessageUpdates(ctx, jid, params)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) NewsletterMarkViewed(ctx context.Context, jid types.JID, serverIDs []types.MessageServerID) error {
	return errmap.ClassifyIQ(r.Client.NewsletterMarkViewed(ctx, jid, serverIDs))
}

func (r RealClient) NewsletterSendReaction(ctx context.Context, jid types.JID, serverID types.MessageServerID, reaction string, messageID types.MessageID) error {
	return errmap.ClassifyIQ(r.Client.NewsletterSendReaction(ctx, jid, serverID, reaction, messageID))
}

func (r RealClient) NewsletterSubscribeLiveUpdates(ctx context.Context, jid types.JID) (time.Duration, error) {
	v0, err := r.Client.NewsletterSubscribeLiveUpdates(ctx, jid)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) PairPhone(ctx context.Context, phone string, showPushNotification bool, clientType wapairing.ClientType, clientDisplayName string) (string, error) {
	v0, err := r.Client.PairPhone(ctx, phone, showPushNotification, clientType, clientDisplayName)
	return v0, errmap.ClassifyIQ(err)
}

func (r RealClient) Logout(ctx context.Context) error {
	return errmap.ClassifyIQ(r.Client.Logout(ctx))
}
