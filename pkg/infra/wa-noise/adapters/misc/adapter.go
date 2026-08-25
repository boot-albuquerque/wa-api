package misc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"wa-api/pkg/infra/wa-noise/adapters/profile"
	waclient "wa-api/pkg/infra/wa-noise/client"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"
	wasession "wa-api/pkg/infra/wa-noise/runtime/session"

	"github.com/rs/zerolog/log"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"

	wa "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/appstate"
	"wa-api/internal/wa-noise/protocol/types"
)

// appStateFetchTimeout é o teto de espera do pull de app-state — mais
// generoso que os 30s de ArchiveChat/RequestUnavailableMessage porque o modo
// "full" pode reprocessar um snapshot inteiro vindo do servidor.
const appStateFetchTimeout = 45 * time.Second

// MiscAdapter implementa ChatOperations, ProfileAccessProvider e
// NewsletterReader sobre o clientManager.
type MiscAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewMiscAdapter cria o adapter com a função de lookup.
func NewMiscAdapter(getClient waclient.Getter) *MiscAdapter {
	return &MiscAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// ArchiveChat arquiva ou desarquiva uma conversa.
//
// O timeout de 30s vinha do use case; ele é característica do transporte
// (SendAppState é uma ida ao servidor), não regra de negócio, e por isso
// desceu junto com a chamada.
func (a *MiscAdapter) ArchiveChat(ctx context.Context, txtID string, chat domain.JID, archive bool) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(chat)
	if err != nil {
		return err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	return client.SendAppState(ctxWithTimeout, appstate.BuildArchive(jid, archive, time.Time{}, nil))
}

// StarMessage stars or unstars a message via app-state patch.
func (a *MiscAdapter) StarMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string, fromMe, star bool) error {
	client, chatJID, err := a.clientAndJID(txtID, chat)
	if err != nil {
		return err
	}
	senderJID, err := wajid.ToJID(sender)
	if err != nil {
		return err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	return client.SendAppState(ctxWithTimeout, appstate.BuildStar(chatJID, senderJID, types.MessageID(messageID), fromMe, star))
}

// PinChat pins or unpins a conversation in the chat list.
//
// Same shape as ArchiveChat: SendAppState with the builder, 30s timeout.
func (a *MiscAdapter) PinChat(ctx context.Context, txtID string, chat domain.JID, pin bool) error {
	client, parsed, err := a.clientAndJID(txtID, chat)
	if err != nil {
		return err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	return client.SendAppState(ctxWithTimeout, appstate.BuildPin(parsed, pin))
}

// MuteChat mutes or unmutes a conversation via app-state patch.
func (a *MiscAdapter) MuteChat(ctx context.Context, txtID string, chat domain.JID, mute bool, muteDuration time.Duration) error {
	client, jid, err := a.clientAndJID(txtID, chat)
	if err != nil {
		return err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	return client.SendAppState(ctxWithTimeout, appstate.BuildMute(jid, mute, muteDuration))
}

// RejectCall rejeita uma chamada recebida.
func (a *MiscAdapter) RejectCall(ctx context.Context, txtID string, from domain.JID, callID string) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	jid, err := wajid.ToJID(from)
	if err != nil {
		return err
	}
	return client.RejectCall(ctx, jid, callID)
}

// RequestUnavailableMessage pede ao par o reenvio de uma mensagem.
func (a *MiscAdapter) RequestUnavailableMessage(ctx context.Context, txtID string, chat, sender domain.JID, messageID string) (domain.UnavailableMessageAck, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}
	chatJID, err := wajid.ToJID(chat)
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}
	senderJID, err := wajid.ToJID(sender)
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}

	unavailableMessage := client.BuildUnavailableMessageRequest(chatJID, senderJID, messageID)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()

	resp, err := client.SendMessage(ctxWithTimeout, chatJID, unavailableMessage, wa.SendRequestExtra{Peer: true})
	if err != nil {
		return domain.UnavailableMessageAck{}, err
	}
	return domain.UnavailableMessageAck{RequestID: resp.ID, Timestamp: resp.Timestamp}, nil
}

// ProfileAccess devolve o acesso ao perfil da sessão.
func (a *MiscAdapter) ProfileAccess(_ context.Context, txtID string) (appport.ProfileDataAccess, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	return profile.NewProfileDataAccessFromInterface(client), nil
}

// ListSubscribed devolve as newsletters assinadas pela sessão.
func (a *MiscAdapter) ListSubscribed(ctx context.Context, txtID string) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetSubscribedNewsletters(ctx)
	if err != nil {
		return nil, err
	}

	newsletter := make([]types.NewsletterMetadata, 0, len(resp))
	for _, info := range resp {
		if info != nil {
			newsletter = append(newsletter, *info)
		}
	}
	return newsletter, nil
}

// SyncContactRoster força o pull do patch de app-state que carrega a agenda
// de contatos (critical_unblock_low). Não mexe em histórico de mensagens —
// capacidade distinta de qualquer fluxo de history sync.
func (a *MiscAdapter) SyncContactRoster(ctx context.Context, txtID string, mode string) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, appStateFetchTimeout)
	defer cancel()

	switch mode {
	case "if_unsynced":
		return client.FetchAppState(ctxWithTimeout, appstate.WAPatchCriticalUnblockLow, false, true)
	case "incremental":
		return client.FetchAppState(ctxWithTimeout, appstate.WAPatchCriticalUnblockLow, false, false)
	case "full":
		return client.FetchAppState(ctxWithTimeout, appstate.WAPatchCriticalUnblockLow, true, false)
	default:
		return apperr.New("invalid_sync_mode", apperr.CategoryValidation,
			fmt.Sprintf("unknown contact roster sync mode %q", mode), false, nil)
	}
}

// SetDisappearingTimer sets the disappearing message timer for a specific
// chat (private or group). The SDK method handles both — the switch on
// chat server is inside the SDK, not here.
func (a *MiscAdapter) SetDisappearingTimer(ctx context.Context, txtID string, chat domain.JID, d time.Duration, at time.Time) error {
	client, parsed, err := a.clientAndJID(txtID, chat)
	if err != nil {
		return err
	}
	return client.SetDisappearingTimer(ctx, parsed, d, at)
}

// SetDefaultDisappearingTimer sets the account-wide default timer.
func (a *MiscAdapter) SetDefaultDisappearingTimer(ctx context.Context, txtID string, d time.Duration) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	return client.SetDefaultDisappearingTimer(ctx, d)
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ChatMuter                      = (*MiscAdapter)(nil)
	_ appport.ChatOperations                 = (*MiscAdapter)(nil)
	_ appport.ChatPinner                     = (*MiscAdapter)(nil)
	_ appport.ProfileAccessProvider          = (*MiscAdapter)(nil)
	_ appport.NewsletterReader               = (*MiscAdapter)(nil)
	_ appport.AppStateSyncer                 = (*MiscAdapter)(nil)
	_ appport.DefaultDisappearingTimerSetter = (*MiscAdapter)(nil)
	_ appport.MessageStarrer                 = (*MiscAdapter)(nil)
)

// --- Newsletters -------------------------------------------------------------
//
// Onze capacidades acrescentadas no levantamento de paridade de 2026-08-20. A
// biblioteca expunha doze e nós expúnhamos UMA (ListSubscribed).
//
// Todas seguem a mesma forma da ListSubscribed acima: obter o cliente da
// sessão, traduzir o JID quando há um, delegar. O que varia é só o método
// chamado — e é por isso que este bloco parece repetitivo: a repetição está no
// protocolo, não no nosso desenho, e escondê-la atrás de uma abstração faria
// cada rota nova ter de descobrir onde encaixar.

// CreateNewsletter cria um canal.
func (a *MiscAdapter) CreateNewsletter(ctx context.Context, txtID, name, description string, picture []byte) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.CreateNewsletter(ctxWithTimeout, wa.CreateNewsletterParams{
		Name: name, Description: description, Picture: picture,
	})
}

// NewsletterInfo devolve os metadados de um canal pelo JID.
func (a *MiscAdapter) NewsletterInfo(ctx context.Context, txtID string, jid domain.JID) (any, error) {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.GetNewsletterInfo(ctxWithTimeout, parsed)
}

// NewsletterInfoWithInvite devolve os metadados pelo CÓDIGO de convite.
//
// Não passa por wajid.ToJID de propósito: o que entra aqui é a chave de um
// link partilhado, não um identificador de conversa. Traduzi-la como JID
// falharia — e falharia com uma mensagem sobre JID inválido, que mandaria quem
// investiga para o lado errado.
func (a *MiscAdapter) NewsletterInfoWithInvite(ctx context.Context, txtID, inviteKey string) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.GetNewsletterInfoWithInvite(ctxWithTimeout, inviteKey)
}

// FollowNewsletter passa a seguir o canal.
func (a *MiscAdapter) FollowNewsletter(ctx context.Context, txtID string, jid domain.JID) error {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	if err := client.FollowNewsletter(ctxWithTimeout, parsed); err != nil {
		return err
	}
	return nil
}

// UnfollowNewsletter deixa de seguir o canal.
func (a *MiscAdapter) UnfollowNewsletter(ctx context.Context, txtID string, jid domain.JID) error {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	if err := client.UnfollowNewsletter(ctxWithTimeout, parsed); err != nil {
		return err
	}
	return nil
}

// ToggleNewsletterMute silencia ou deixa de silenciar o canal.
func (a *MiscAdapter) ToggleNewsletterMute(ctx context.Context, txtID string, jid domain.JID, mute bool) error {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	if err := client.NewsletterToggleMute(ctxWithTimeout, parsed, mute); err != nil {
		return err
	}
	return nil
}

// NewsletterMessages devolve mensagens do canal, paginadas para trás.
//
// `count` e `before` a zero produzem params com campos zerados, e a biblioteca
// OMITE o atributo correspondente nesse caso (capabilities/newsletter,
// messagesAttrs) — deixando o servidor usar o padrão dele. É por isso que não
// inventamos um limite aqui: escolher um por omissão seria decidir em nome do
// WhatsApp.
func (a *MiscAdapter) NewsletterMessages(ctx context.Context, txtID string, jid domain.JID, count int, before string) (any, error) {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.GetNewsletterMessages(ctxWithTimeout, parsed, &wa.GetNewsletterMessagesParams{
		Count: count, Before: types.MessageServerID(serverIDFromText(before)),
	})
}

// NewsletterMessageUpdates devolve ATUALIZAÇÕES de mensagens já vistas.
func (a *MiscAdapter) NewsletterMessageUpdates(ctx context.Context, txtID string, jid domain.JID, count int, since time.Time, after string) (any, error) {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return nil, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.GetNewsletterMessageUpdates(ctxWithTimeout, parsed, &wa.GetNewsletterUpdatesParams{
		Count: count, Since: since, After: types.MessageServerID(serverIDFromText(after)),
	})
}

// MarkNewsletterViewed marca mensagens como vistas.
func (a *MiscAdapter) MarkNewsletterViewed(ctx context.Context, txtID string, jid domain.JID, serverIDs []int) error {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return err
	}
	ids := make([]types.MessageServerID, 0, len(serverIDs))
	for _, id := range serverIDs {
		ids = append(ids, types.MessageServerID(id))
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	if err := client.NewsletterMarkViewed(ctxWithTimeout, parsed, ids); err != nil {
		return err
	}
	return nil
}

// SendNewsletterReaction reage a uma mensagem do canal.
func (a *MiscAdapter) SendNewsletterReaction(ctx context.Context, txtID string, jid domain.JID, serverID int, reaction, messageID string) error {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	if err := client.NewsletterSendReaction(ctxWithTimeout, parsed, types.MessageServerID(serverID), reaction, types.MessageID(messageID)); err != nil {
		return err
	}
	return nil
}

// SubscribeNewsletterLiveUpdates liga as atualizações ao vivo.
func (a *MiscAdapter) SubscribeNewsletterLiveUpdates(ctx context.Context, txtID string, jid domain.JID) (time.Duration, error) {
	client, parsed, err := a.clientAndJID(txtID, jid)
	if err != nil {
		return 0, err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.NewsletterSubscribeLiveUpdates(ctxWithTimeout, parsed)
}

// DemoteNewsletterAdmin demotes a channel admin to subscriber.
func (a *MiscAdapter) DemoteNewsletterAdmin(ctx context.Context, txtID string, channelJID, userJID domain.JID) error {
	client, parsedChannel, err := a.clientAndJID(txtID, channelJID)
	if err != nil {
		return err
	}
	parsedUser, err := wajid.ToJID(userJID)
	if err != nil {
		return err
	}
	parsedUser = resolveToLID(ctx, client, parsedUser, "demote")
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.NewsletterDemoteAdmin(ctxWithTimeout, parsedChannel, parsedUser)
}

// ChangeNewsletterOwner transfers channel ownership.
func (a *MiscAdapter) ChangeNewsletterOwner(ctx context.Context, txtID string, channelJID, newOwnerJID domain.JID) error {
	client, parsedChannel, err := a.clientAndJID(txtID, channelJID)
	if err != nil {
		return err
	}
	parsedOwner, err := wajid.ToJID(newOwnerJID)
	if err != nil {
		return err
	}
	parsedOwner = resolveToLID(ctx, client, parsedOwner, "change_owner")
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.NewsletterChangeOwner(ctxWithTimeout, parsedChannel, parsedOwner)
}

// DeleteNewsletter permanently deletes a channel (IRREVERSIBLE).
func (a *MiscAdapter) DeleteNewsletter(ctx context.Context, txtID string, channelJID domain.JID) error {
	client, parsed, err := a.clientAndJID(txtID, channelJID)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, waclient.RequestTimeout)
	defer cancel()
	return client.NewsletterDelete(ctxWithTimeout, parsed)
}

// resolveToLID converts a PN JID to its LID form via the client's store.
// DIRECTIONAL: only converts PN→LID. If the JID is already LID, it is
// returned unchanged — converting LID→PN would break the request (F228,
// F233c: measured 2026-08-25).
//
// If the store has no mapping, the original JID is kept and a warning is
// logged. The caller can proceed — the server will reject with a meaningful
// error instead of a silent failure.
func resolveToLID(ctx context.Context, client waclient.Client, jid types.JID, operation string) types.JID {
	if jid.Server != types.DefaultUserServer {
		return jid
	}
	st := client.Store()
	if st == nil {
		log.Warn().
			Str("jid", jid.String()).
			Str("operation", operation).
			Msg("no store available; cannot resolve PN to LID")
		return jid
	}
	alt, err := st.GetAltJID(ctx, jid)
	if err != nil || alt.IsEmpty() {
		log.Warn().
			Str("jid", jid.String()).
			Str("operation", operation).
			Msg("PN could not be resolved to LID; sending as-is")
		return jid
	}
	log.Info().
		Str("original", jid.String()).
		Str("resolved", alt.String()).
		Str("operation", operation).
		Msg("resolved PN to LID for newsletter admin operation")
	return alt
}

// clientAndJID resolve o cliente da sessão e o JID de uma vez.
//
// Existe porque onze métodos acima começavam com as mesmas seis linhas, e a
// ordem importa: o cliente PRIMEIRO. Traduzir o JID antes de saber se há sessão
// faria um pedido sem sessão falhar com "JID inválido" quando o problema é
// outro — a mesma classe de erro que a HOUSEKEEP F94 nomeia.
func (a *MiscAdapter) clientAndJID(txtID string, jid domain.JID) (waclient.Client, types.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, types.JID{}, err
	}
	parsed, err := wajid.ToJID(jid)
	if err != nil {
		return nil, types.JID{}, err
	}
	return client, parsed, nil
}

// serverIDFromText converte o id de servidor recebido como texto.
//
// Devolve 0 quando não é número, e 0 é exatamente o que a biblioteca trata como
// "omitir o atributo". Recusar seria inventar uma validação que o protocolo não
// faz — o servidor é que decide se o valor lhe serve.
func serverIDFromText(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	return n
}

// SetStatusMessage publishes the account status ("about") text.
//
// Não passa por clientAndJID porque não há JID nenhum em jogo: o estado é da
// CONTA, não de uma conversa. Pedir um JID aqui obrigaria o chamador a
// inventar um.
func (a *MiscAdapter) SetStatusMessage(ctx context.Context, txtID, msg string) error {
	client, err := a.Client(txtID)
	if err != nil {
		return err
	}
	if err := client.SetStatusMessage(ctx, msg); err != nil {
		return err
	}
	return nil
}

// RequestHistorySync monta o pedido de histórico e envia-o ao dispositivo
// principal.
//
// A ORDEM importa e é a que a biblioteca documenta: `BuildHistorySyncRequest`
// monta, `SendPeerMessage` envia. A resposta NÃO vem aqui — chega depois como
// um evento `HistorySync` do tipo ON_DEMAND, e é o handler de eventos que a
// grava. Por isso o retorno é só o id do PEDIDO: prometer contagem de
// mensagens na resposta desta chamada seria mentir sobre um dado que ainda não
// existe.
func (a *MiscAdapter) RequestHistorySync(ctx context.Context, txtID string, anchor appport.HistoryAnchor, count int) (string, error) {
	client, parsed, err := a.clientAndJID(txtID, anchor.ChatJID)
	if err != nil {
		return "", err
	}

	info := &types.MessageInfo{
		ID:        types.MessageID(anchor.MessageID),
		Timestamp: time.Unix(anchor.Timestamp, 0),
		MessageSource: types.MessageSource{
			Chat:     parsed,
			IsFromMe: anchor.FromMe,
		},
	}

	pedido := client.BuildHistorySyncRequest(info, count)
	if pedido == nil {
		return "", apperr.New("history_sync_build_failed", apperr.CategoryInternal,
			"could not build the history sync request", false, nil)
	}

	resp, err := client.SendPeerMessage(ctx, pedido)
	if err != nil {
		return "", err
	}
	return string(resp.ID), nil
}
