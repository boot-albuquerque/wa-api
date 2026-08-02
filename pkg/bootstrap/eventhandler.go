package bootstrap

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types/events"
)

// eventState carrega o estado que os ramos do type-switch de myEventHandler
// compartilhavam entre si quando eram todos o corpo de uma função só. Cada
// handler extraído recebe um *eventState e o preenche exatamente como o ramo
// correspondente preenchia as variáveis locais antes da Fase 7.
//
// Handlers que devolvem bool traduzem o `return` que o ramo original tinha:
// false significa "aborte sem disparar webhook", que é o que aquele `return`
// fazia ao sair de myEventHandler antes do bloco final.
type eventState struct {
	txtid     string
	postmap   map[string]interface{}
	dowebhook int
}

func (mycli *MyClient) myEventHandler(rawEvt interface{}) {
	st := &eventState{
		txtid:   mycli.UserID,
		postmap: map[string]interface{}{"event": rawEvt},
	}
	// `path` nunca é escrito por ramo nenhum do switch — sempre chega vazio em
	// sendEventWithWebHook. Mantido como estava, e por isso fora de
	// eventState: um campo que ninguém escreve seria pior que a variável.
	path := ""

	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		mycli.handleAppStateSyncComplete(evt, st)
	case *events.Connected, *events.PushNameSetting:
		if !mycli.handleConnected(st) {
			return
		}
	case *events.PairSuccess:
		if !mycli.handlePairSuccess(evt, st) {
			return
		}
	case *events.StreamReplaced:
		if !mycli.handleStreamReplaced(evt, st) {
			return
		}
	case *events.Message:
		mycli.handleMessage(evt, st)
	case *events.Receipt:
		if !mycli.handleReceipt(evt, st) {
			return
		}
	case *events.Presence:
		mycli.handlePresence(evt, st)
	case *events.HistorySync:
		mycli.handleHistorySync(evt, st)
	case *events.AppState:
		mycli.handleAppState(evt, st)
	case *events.LoggedOut:
		// O `defer` fica AQUI, e nao dentro de handleLoggedOut: defer adia ate
		// o fim da funcao, entao no arquivo original o sinal saia DEPOIS de
		// sendEventWithWebHook. Move-lo para o handler o anteciparia.
		defer func() {
			// Use a non-blocking send to prevent a deadlock if the receiver has already terminated.
			appCtx.KillChannel.Signal(mycli.UserID)
		}()
		if !mycli.handleLoggedOut(evt, st) {
			return
		}
	case *events.ChatPresence:
		mycli.handleChatPresence(evt, st)
	case *events.CallOffer:
		mycli.handleCallOffer(evt, st)
	case *events.CallAccept:
		mycli.handleCallAccept(evt, st)
	case *events.CallTerminate:
		mycli.handleCallTerminate(evt, st)
	case *events.CallOfferNotice:
		mycli.handleCallOfferNotice(evt, st)
	case *events.CallRelayLatency:
		mycli.handleCallRelayLatency(evt, st)
	case *events.Disconnected:
		mycli.handleDisconnected(evt, st)
	case *events.ConnectFailure:
		mycli.handleConnectFailure(evt, st)
	case *events.UndecryptableMessage:
		mycli.handleUndecryptableMessage(evt, st)
	case *events.MediaRetry:
		mycli.handleMediaRetry(evt, st)
	case *events.GroupInfo:
		mycli.handleGroupInfo(evt, st)
	case *events.JoinedGroup:
		mycli.handleJoinedGroup(evt, st)
	case *events.Picture:
		mycli.handlePicture(evt, st)
	case *events.BlocklistChange:
		mycli.handleBlocklistChange(evt, st)
	case *events.Blocklist:
		mycli.handleBlocklist(evt, st)
	case *events.KeepAliveRestored:
		mycli.handleKeepAliveRestored(evt, st)
	case *events.KeepAliveTimeout:
		mycli.handleKeepAliveTimeout(evt, st)
	case *events.ClientOutdated:
		mycli.handleClientOutdated(evt, st)
	case *events.TemporaryBan:
		mycli.handleTemporaryBan(evt, st)
	case *events.StreamError:
		mycli.handleStreamError(evt, st)
	case *events.PairError:
		mycli.handlePairError(evt, st)
	case *events.PrivacySettings:
		mycli.handlePrivacySettings(evt, st)
	case *events.UserAbout:
		mycli.handleUserAbout(evt, st)
	case *events.OfflineSyncCompleted:
		mycli.handleOfflineSyncCompleted(evt, st)
	case *events.OfflineSyncPreview:
		mycli.handleOfflineSyncPreview(evt, st)
	case *events.IdentityChange:
		mycli.handleIdentityChange(evt, st)
	case *events.NewsletterJoin:
		mycli.handleNewsletterJoin(evt, st)
	case *events.NewsletterLeave:
		mycli.handleNewsletterLeave(evt, st)
	case *events.NewsletterMuteChange:
		mycli.handleNewsletterMuteChange(evt, st)
	case *events.NewsletterLiveUpdate:
		mycli.handleNewsletterLiveUpdate(evt, st)
	case *events.FBMessage:
		mycli.handleFBMessage(evt, st)
	default:
		log.Warn().Str("event", fmt.Sprintf("%+v", evt)).Msg("Unhandled event")
	}

	if st.dowebhook == 1 {
		sendEventWithWebHook(mycli, st.postmap, path)
	}
}
