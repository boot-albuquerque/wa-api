package retry

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"wa-api/internal/noise/protocol/proto/waE2E"
	"wa-api/internal/noise/protocol/proto/waMsgApplication"
	"wa-api/internal/noise/protocol/types"
	"wa-api/internal/noise/protocol/types/events"
)

// RecentMessage e' uma mensagem enviada guardada para atender um eventual
// recibo de retry. So' um dos dois campos e' preenchido: WA para o protocolo
// WhatsApp, FB para o de Messenger.
//
// Os campos eram nao exportados (wa/fb) antes da extracao. Exporta-los e'
// alargamento da superficie usada por internals.go (o tipo antes so' tinha
// campos privados, logo era inutilizavel de fora), nao quebra: a raiz mantem
// `type RecentMessage = retry.RecentMessage`, um APELIDO, para que internals.go
// — gerado, F29, fora do escopo — compile sem ser tocado.
type RecentMessage struct {
	WA *waE2E.Message
	FB *waMsgApplication.MessageApplication
}

// IsEmpty diz se nenhum dos dois protocolos foi preenchido.
func (rm RecentMessage) IsEmpty() bool {
	return rm.WA == nil && rm.FB == nil
}

// AddRecent guarda uma mensagem recem-enviada para um eventual retry.
//
// Quando UseMessageStore esta' ligado, a mensagem tambem e' serializada no
// store persistente ANTES de entrar no cache em memoria — e um erro ali aborta
// a funcao, ou seja, a mensagem nao entra no cache. Ordem do upstream,
// preservada: o call site (send.go) propaga esse erro.
//
// O expurgo do store de retry e' throttled por StoreClearInterval: AddRecent
// carimba State.MarkStoreCleared depois de cada DeleteOldOutgoingEvents bem
// sucedido. Ate' a correcao da F52 o carimbo nao existia e o expurgo rodava em
// TODA gravacao.
func AddRecent(
	ctx context.Context,
	t Transport,
	to types.JID,
	id types.MessageID,
	wa *waE2E.Message,
	fb *waMsgApplication.MessageApplication,
) error {
	if t.UseMessageStore() {
		var buf []byte
		var format string
		var err error
		if wa != nil {
			buf, err = proto.Marshal(wa)
			format = StoreFormatWA
		} else if fb != nil {
			buf, err = proto.Marshal(fb)
			format = StoreFormatFB
		}
		if err != nil {
			return fmt.Errorf("failed to marshal message for retry store: %w", err)
		}
		if buf != nil {
			err = t.Store().EventBuffer.AddOutgoingEvent(ctx, to, id, format, buf)
			if err != nil {
				return fmt.Errorf("failed to add message to retry store: %w", err)
			}
			if time.Since(t.State().LastStoreClear()) > StoreClearInterval {
				err = t.Store().EventBuffer.DeleteOldOutgoingEvents(ctx)
				if err != nil {
					return fmt.Errorf("failed to clear old messages from retry store: %w", err)
				}
				// O carimbo que faltava. Sem ele o time.Since acima era sempre
				// maior que StoreClearInterval e o expurgo rodava em toda
				// gravacao — o throttle inteiro era decorativo (F52).
				t.State().MarkStoreCleared(time.Now())
			}
		}
	}
	t.State().AddRecent(RecentKey{to, id}, RecentMessage{WA: wa, FB: fb})
	return nil
}

// GetRecent le' o cache em memoria de mensagens enviadas.
func GetRecent(t Transport, to types.JID, id types.MessageID) RecentMessage {
	return t.State().GetRecent(RecentKey{to, id})
}

// GetForRetry procura a mensagem original de um recibo de retry, nesta ordem:
// cache em memoria pelo JID do chat; cache pelo OUTRO lado do par LID/PN;
// store persistente (se ligado); e por fim o callback GetMessageForRetry.
//
// O ramo do store e' terminal: quando UseMessageStore esta' ligado, o callback
// NAO e' consultado, nem mesmo se o store nao achou nada. Comportamento do
// upstream, preservado.
//
// Devolver (nil, nil) significa "nao achei" e nao e' erro — quem chama trata.
func GetForRetry(
	ctx context.Context,
	t Transport,
	receipt *events.Receipt,
	messageID types.MessageID,
) (*RecentMessage, error) {
	msg := GetRecent(t, receipt.Chat, messageID)
	if !msg.IsEmpty() {
		t.Log().Debugf("Found message in local cache to accept retry receipt for %s/%s from %s", receipt.Chat, messageID, receipt.Sender)
		return &msg, nil
	}
	var altChat types.JID
	var err error
	switch receipt.Chat.Server {
	case types.DefaultUserServer:
		altChat, err = t.Store().LIDs.GetLIDForPN(ctx, receipt.Chat)
	case types.HiddenUserServer:
		altChat, err = t.Store().LIDs.GetPNForLID(ctx, receipt.Chat)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alternate JID for %s: %w", receipt.Chat, err)
	} else if !altChat.IsEmpty() {
		msg = GetRecent(t, altChat, messageID)
		if !msg.IsEmpty() {
			t.Log().Debugf("Found message in local cache with alternate chat JID %s to accept retry receipt for %s/%s from %s", altChat, receipt.Chat, messageID, receipt.Sender)
			return &msg, nil
		}
	}
	if t.UseMessageStore() {
		format, buf, err := t.Store().EventBuffer.GetOutgoingEvent(ctx, receipt.Chat, altChat, messageID)
		if err != nil {
			return nil, fmt.Errorf("failed to get message from retry store: %w", err)
		}
		return ParseRecent(format, buf)
	}
	waMsg := t.GetMessageForRetry(receipt.Sender, receipt.Chat, messageID)
	if waMsg != nil {
		t.Log().Debugf("Found message in GetMessageForRetry to accept retry receipt for %s/%s from %s", receipt.Chat, messageID, receipt.Sender)
		return &RecentMessage{WA: waMsg}, nil
	}
	return nil, nil
}

// ParseRecent desserializa uma mensagem lida do store de retry. O formato tem
// de ser um dos dois que AddRecent grava.
func ParseRecent(format string, buf []byte) (*RecentMessage, error) {
	var rm RecentMessage
	var err error
	switch format {
	case StoreFormatWA:
		rm.WA = &waE2E.Message{}
		err = proto.Unmarshal(buf, rm.WA)
	case StoreFormatFB:
		rm.FB = &waMsgApplication.MessageApplication{}
		err = proto.Unmarshal(buf, rm.FB)
	default:
		err = fmt.Errorf("unknown format in retry store: %s", format)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload in retry store: %w", err)
	}
	return &rm, nil
}
