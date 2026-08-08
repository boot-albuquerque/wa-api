package bootstrap

import (
	"fmt"

	"wa-api/internal/wa-noise/protocol/types/events"

	"github.com/rs/zerolog/log"
)

// eventState carrega o estado que os ramos do type-switch de handleEvent
// compartilhavam entre si quando eram todos o corpo de uma função só. Cada
// handler extraído recebe um *eventState e o preenche exatamente como o ramo
// correspondente preenchia as variáveis locais antes da Fase 7.
//
// Handlers que devolvem bool traduzem o `return` que o ramo original tinha:
// false significa "aborte sem disparar webhook", que é o que aquele `return`
// fazia ao sair de handleEvent antes do bloco final.
type eventState struct {
	txtid     string
	postmap   map[string]interface{}
	dowebhook int
}

func (evh *UserEventHandler) handleEvent(rawEvt interface{}) {
	st := &eventState{
		txtid:   evh.UserID,
		postmap: map[string]interface{}{"event": rawEvt},
	}
	// `path` nunca é escrito por ramo nenhum do switch — sempre chega vazio em
	// sendEventWithWebHook. Mantido como estava, e por isso fora de
	// eventState: um campo que ninguém escreve seria pior que a variável.
	path := ""

	switch evt := rawEvt.(type) {
	case *events.AppStateSyncComplete:
		evh.handleAppStateSyncComplete(evt, st)
	// PushName e BusinessName anunciam que um CONTATO mudou de nome — o SDK
	// ja' persistiu o dado (inclusive o par LID<->PN) antes de emitir, entao
	// nao ha o que fazer aqui hoje. O `case` existe para que "Unhandled
	// event" volte a significar "apareceu algo que nao previmos", e nao
	// "apareceu algo que decidimos ignorar": sem ele, os dois casos se
	// misturam no mesmo warn e o aviso perde o valor que tinha.
	//
	// Se um dia esses eventos virarem webhook, e' aqui que entram. Ver F73.
	case *events.PushName, *events.BusinessName:
		return
	case *events.Connected, *events.PushNameSetting:
		if !evh.handleConnected(st) {
			return
		}
	case *events.PairSuccess:
		if !evh.handlePairSuccess(evt, st) {
			return
		}
	case *events.StreamReplaced:
		if !evh.handleStreamReplaced(evt, st) {
			return
		}
	case *events.Message:
		evh.handleMessage(evt, st)
	case *events.Receipt:
		if !evh.handleReceipt(evt, st) {
			return
		}
	case *events.Presence:
		evh.handlePresence(evt, st)
	case *events.HistorySync:
		evh.handleHistorySync(evt, st)
	case *events.AppState:
		evh.handleAppState(evt, st)
	case *events.LoggedOut:
		// O `defer` fica AQUI, e nao dentro de handleLoggedOut: defer adia ate
		// o fim da funcao, entao no arquivo original o sinal saia DEPOIS de
		// sendEventWithWebHook. Move-lo para o handler o anteciparia.
		defer func() {
			// Use a non-blocking send to prevent a deadlock if the receiver has already terminated.
			appCtx.KillChannel.Signal(evh.UserID)
		}()
		if !evh.handleLoggedOut(evt, st) {
			return
		}
	case *events.ChatPresence:
		evh.handleChatPresence(evt, st)
	case *events.CallOffer:
		evh.handleCallOffer(evt, st)
	case *events.CallAccept:
		evh.handleCallAccept(evt, st)
	case *events.CallTerminate:
		evh.handleCallTerminate(evt, st)
	case *events.CallOfferNotice:
		evh.handleCallOfferNotice(evt, st)
	case *events.CallRelayLatency:
		evh.handleCallRelayLatency(evt, st)
	case *events.Disconnected:
		evh.handleDisconnected(evt, st)
	case *events.ConnectFailure:
		evh.handleConnectFailure(evt, st)
	case *events.UndecryptableMessage:
		evh.handleUndecryptableMessage(evt, st)
	case *events.MediaRetry:
		evh.handleMediaRetry(evt, st)
	case *events.GroupInfo:
		evh.handleGroupInfo(evt, st)
	case *events.JoinedGroup:
		evh.handleJoinedGroup(evt, st)
	case *events.Picture:
		evh.handlePicture(evt, st)
	case *events.BlocklistChange:
		evh.handleBlocklistChange(evt, st)
	case *events.Blocklist:
		evh.handleBlocklist(evt, st)
	case *events.KeepAliveRestored:
		evh.handleKeepAliveRestored(evt, st)
	case *events.KeepAliveTimeout:
		evh.handleKeepAliveTimeout(evt, st)
	case *events.ClientOutdated:
		evh.handleClientOutdated(evt, st)
	case *events.TemporaryBan:
		evh.handleTemporaryBan(evt, st)
	case *events.StreamError:
		evh.handleStreamError(evt, st)
	case *events.PairError:
		evh.handlePairError(evt, st)
	case *events.PrivacySettings:
		evh.handlePrivacySettings(evt, st)
	case *events.UserAbout:
		evh.handleUserAbout(evt, st)
	case *events.OfflineSyncCompleted:
		evh.handleOfflineSyncCompleted(evt, st)
	case *events.OfflineSyncPreview:
		evh.handleOfflineSyncPreview(evt, st)
	case *events.IdentityChange:
		evh.handleIdentityChange(evt, st)
	case *events.NewsletterJoin:
		evh.handleNewsletterJoin(evt, st)
	case *events.NewsletterLeave:
		evh.handleNewsletterLeave(evt, st)
	case *events.NewsletterMuteChange:
		evh.handleNewsletterMuteChange(evt, st)
	case *events.NewsletterLiveUpdate:
		evh.handleNewsletterLiveUpdate(evt, st)
	case *events.FBMessage:
		evh.handleFBMessage(evt, st)
	default:
		log.Warn().Str("event", fmt.Sprintf("%+v", evt)).Msg("Unhandled event")
	}

	if st.dowebhook == 1 {
		sendEventWithWebHook(evh, st.postmap, path)
	}
}
