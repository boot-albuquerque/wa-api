package bootstrap

import (
	"fmt"

	"wa-api/internal/wa-noise/protocol/types/events"

	"github.com/rs/zerolog/log"
	"reflect"
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
	// QR NAO pode cair no `default`: o ramo default dumpa a struct inteira, e
	// `Codes` sao os codigos de pareamento. Um deles, lido no log, vincula um
	// aparelho a conta — e' credencial, nao diagnostico. Foram 6 codigos e
	// 1678 bytes num unico log.Warn no pareamento medido em 2026-08-07. Ver
	// F76.
	//
	// A entrega do QR ao cliente nao passa por aqui: quem a faz e'
	// pkg/infra/wa-noise/runtime/session/events.go:54, pelo canal da sessao.
	// Este ramo so' existia como log.
	//
	// Fica em Debug, e nao em Info, porque o evento e' rotineiro (um por
	// Connect nao autenticado) e a contagem so' serve quando se esta' olhando
	// o pareamento de perto. O que importa e' que nem o codigo nem o dump
	// saem em nivel nenhum.
	case *events.QR:
		log.Debug().Int("codes", len(evt.Codes)).Msg("QR codes recebidos")
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
		// Nome do TIPO e nomes dos CAMPOS, nunca os valores (F91).
		//
		// Este ramo logava `%+v` da struct inteira. A F76 pegou o caso mais
		// grave — o evento QR carrega `Codes`, que são códigos de PAREAMENTO,
		// e quem os lê vincula um aparelho à conta — e foi corrigida com um
		// `case` dedicado para QR. Mas isso tratou a instância, não a classe:
		// qualquer OUTRO tipo que caia aqui carregando dado sensível vaza
		// igual, e já se viu no log evento de chamada com CallID e
		// CallCreator completos.
		//
		// O aviso não perde valor: `%T` identifica o tipo com precisão maior
		// que o dump, e os nomes de campo mostram a forma de quem for
		// implementar o `case` que falta.
		log.Warn().
			Str("event_type", fmt.Sprintf("%T", evt)).
			Strs("fields", unhandledEventFields(evt)).
			Msg("Unhandled event")
	}

	if st.dowebhook == 1 {
		sendEventWithWebHook(evh, st.postmap, path)
	}
}

// unhandledEventFields returns an event's FIELD NAMES, never their values.
//
// This is what keeps the `default` branch above from leaking. Values are the
// hazard: pairing codes, call identifiers, message contents. Field names carry
// the shape a developer needs in order to write the missing `case`, and carry
// no secret.
//
// Nil, non-struct or unnamed types return nil rather than guessing — an empty
// field list is honest, a fabricated one is not.
func unhandledEventFields(evt any) []string {
	value := reflect.ValueOf(evt)
	for value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil
	}

	eventType := value.Type()
	names := make([]string, 0, eventType.NumField())
	for i := 0; i < eventType.NumField(); i++ {
		names = append(names, eventType.Field(i).Name)
	}
	return names
}
