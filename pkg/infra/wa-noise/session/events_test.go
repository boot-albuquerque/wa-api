package session

import (
	"testing"

	appport "wa-api/pkg/application/contracts"

	whatsmeow "wa-api/internal/wa-noise/core"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

func TestWhatsmeowSession_Subscribe(t *testing.T) {
	var handler whatsmeow.EventHandler
	removed := false
	client := &fakeSessionClient{
		AddEventHandlerFn: func(h whatsmeow.EventHandler) uint32 {
			handler = h
			return 42
		},
		RemoveHandlerFn: func(id uint32) bool {
			if id != 42 {
				t.Errorf("RemoveEventHandler id = %d, want 42", id)
			}
			removed = true
			return true
		},
	}
	s := &whatsmeowSession{device: &store.Device{}, client: client}

	var got []appport.SessionEvent
	unsubscribe, err := s.Subscribe(func(evt appport.SessionEvent) {
		got = append(got, evt)
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if handler == nil {
		t.Fatal("expected AddEventHandler to receive a handler")
	}

	handler(&events.Connected{})
	handler(&events.Disconnected{})
	handler(&events.LoggedOut{Reason: events.ConnectFailureLoggedOut})
	handler(&events.PairSuccess{ID: types.NewJID("5511999999999", types.DefaultUserServer), BusinessName: "biz", Platform: "android"})
	handler(&events.QR{Codes: []string{"code1", "code2"}})
	handler(&events.StreamReplaced{})
	handler("not an event we care about")

	if len(got) != 6 {
		t.Fatalf("got %d events, want 6: %+v", len(got), got)
	}
	if got[0].Kind != appport.SessionEventKindConnected {
		t.Errorf("event 0 kind = %s", got[0].Kind)
	}
	if got[1].Kind != appport.SessionEventKindDisconnected || got[1].Disconnected == nil {
		t.Errorf("event 1 = %+v", got[1])
	}
	if got[2].Kind != appport.SessionEventKindLoggedOut || got[2].LoggedOut == nil {
		t.Errorf("event 2 = %+v", got[2])
	}
	if got[3].Kind != appport.SessionEventKindPairSuccess || got[3].PairSuccess == nil || got[3].PairSuccess.BusinessName != "biz" {
		t.Errorf("event 3 = %+v", got[3])
	}
	if got[4].Kind != appport.SessionEventKindQR || got[4].QR == nil || got[4].QR.Code != "code1" {
		t.Errorf("event 4 = %+v", got[4])
	}
	if got[5].Kind != appport.SessionEventKindStreamReplaced {
		t.Errorf("event 5 = %+v", got[5])
	}

	unsubscribe()
	unsubscribe() // idempotent
	if !removed {
		t.Error("expected RemoveEventHandler to be called")
	}
}

func TestWhatsmeowSession_Subscribe_NilHandler(t *testing.T) {
	s := &whatsmeowSession{device: &store.Device{}, client: &fakeSessionClient{}}
	if _, err := s.Subscribe(nil); err == nil {
		t.Error("expected error subscribing with nil handler")
	}
}
