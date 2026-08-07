package session

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"
)

func TestWaNoiseSession_Pair_AlreadyHasCredentials(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	s := &wanoiseSession{device: &store.Device{ID: &jid}, client: &fakeSessionClient{}}
	if _, err := s.Pair(context.Background()); err == nil {
		t.Error("expected error pairing a session that already has credentials")
	}
}

func TestWaNoiseSession_Pair_TranslatesEvents(t *testing.T) {
	items := make(chan wanoise.QRChannelItem, 3)
	items <- wanoise.QRChannelItem{Event: wanoise.QRChannelEventCode, Code: "abc", Timeout: 20 * time.Second}
	items <- wanoise.QRChannelItem{Event: "timeout"}
	items <- wanoise.QRChannelItem{Event: "success"}
	close(items)

	connected := false
	client := &fakeSessionClient{
		GetQRChannelFn: func(ctx context.Context) (<-chan wanoise.QRChannelItem, error) {
			return items, nil
		},
		ConnectFn: func() error {
			connected = true
			return nil
		},
	}
	s := &wanoiseSession{device: &store.Device{}, client: client}

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

func TestWaNoiseSession_Pair_GetQRChannelError(t *testing.T) {
	client := &fakeSessionClient{
		GetQRChannelFn: func(ctx context.Context) (<-chan wanoise.QRChannelItem, error) {
			return nil, errors.New("boom")
		},
	}
	s := &wanoiseSession{device: &store.Device{}, client: client}
	if _, err := s.Pair(context.Background()); err == nil {
		t.Error("expected error when GetQRChannel fails")
	}
}
