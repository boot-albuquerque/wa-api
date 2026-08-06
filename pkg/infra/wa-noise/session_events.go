package whatsmeow

import (
	"fmt"
	"sync"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"

	"wa-api/internal/wa-noise/types/events"
)

// Subscribe traduz os 6 eventos de sessão/transporte do whatsmeow para
// appport.SessionEvent. Eventos de domínio (mensagem, presença, grupo) não
// passam por aqui: seguem no handler registrado via SessionAttachHook.
func (s *whatsmeowSession) Subscribe(fn func(appport.SessionEvent)) (func(), error) {
	if fn == nil {
		return nil, apperr.New("session_subscribe_nil_handler", apperr.CategoryValidation, "subscriber must not be nil", false, nil)
	}

	id := s.client.AddEventHandler(func(raw any) {
		if evt, ok := translateSessionEvent(raw); ok {
			fn(evt)
		}
	})

	var once sync.Once
	return func() { once.Do(func() { s.client.RemoveEventHandler(id) }) }, nil
}

func translateSessionEvent(raw any) (appport.SessionEvent, bool) {
	switch evt := raw.(type) {
	case *events.Connected:
		return appport.SessionEvent{Kind: appport.SessionEventKindConnected}, true
	case *events.Disconnected:
		return appport.SessionEvent{
			Kind:         appport.SessionEventKindDisconnected,
			Disconnected: &appport.SessionDisconnectedEvent{Reason: fmt.Sprintf("%+v", evt)},
		}, true
	case *events.LoggedOut:
		return appport.SessionEvent{
			Kind:      appport.SessionEventKindLoggedOut,
			LoggedOut: &appport.SessionLoggedOutEvent{Reason: evt.Reason.String()},
		}, true
	case *events.PairSuccess:
		return appport.SessionEvent{
			Kind: appport.SessionEventKindPairSuccess,
			PairSuccess: &appport.SessionPairSuccessEvent{
				JID:          evt.ID.String(),
				BusinessName: evt.BusinessName,
				Platform:     evt.Platform,
			},
		}, true
	case *events.QR:
		qr := &appport.SessionQREvent{}
		if len(evt.Codes) > 0 {
			qr.Code = evt.Codes[0]
		}
		return appport.SessionEvent{Kind: appport.SessionEventKindQR, QR: qr}, true
	case *events.StreamReplaced:
		return appport.SessionEvent{Kind: appport.SessionEventKindStreamReplaced}, true
	default:
		return appport.SessionEvent{}, false
	}
}
