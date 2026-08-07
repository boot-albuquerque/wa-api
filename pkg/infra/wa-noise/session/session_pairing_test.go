package session

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"

	whatsmeow "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

func TestWhatsmeowSession_Pair_AlreadyHasCredentials(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	s := &whatsmeowSession{device: &store.Device{ID: &jid}, client: &fakeSessionClient{}}
	if _, err := s.Pair(context.Background()); err == nil {
		t.Error("expected error pairing a session that already has credentials")
	}
}

func TestWhatsmeowSession_Pair_TranslatesEvents(t *testing.T) {
	items := make(chan whatsmeow.QRChannelItem, 3)
	items <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "abc", Timeout: 20 * time.Second}
	items <- whatsmeow.QRChannelItem{Event: "timeout"}
	items <- whatsmeow.QRChannelItem{Event: "success"}
	close(items)

	connected := false
	client := &fakeSessionClient{
		GetQRChannelFn: func(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
			return items, nil
		},
		ConnectFn: func() error {
			connected = true
			return nil
		},
	}
	s := &whatsmeowSession{device: &store.Device{}, client: client}

	out, err := s.Pair(context.Background())
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if !connected {
		t.Error("expected Connect to be called")
	}

	var got []appport.PairingEvent
	for evt := range out {
		got = append(got, evt)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].Kind != appport.PairingEventKindQR || got[0].Code != "abc" {
		t.Errorf("event 0 = %+v", got[0])
	}
	if got[1].Kind != appport.PairingEventKindTimeout {
		t.Errorf("event 1 = %+v", got[1])
	}
	if got[2].Kind != appport.PairingEventKindSuccess {
		t.Errorf("event 2 = %+v", got[2])
	}
}

func TestWhatsmeowSession_Pair_GetQRChannelError(t *testing.T) {
	client := &fakeSessionClient{
		GetQRChannelFn: func(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
			return nil, errors.New("boom")
		},
	}
	s := &whatsmeowSession{device: &store.Device{}, client: client}
	if _, err := s.Pair(context.Background()); err == nil {
		t.Error("expected error when GetQRChannel fails")
	}
}
