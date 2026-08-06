package whatsmeow

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "wa-api/pkg/application/contracts"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"golang.org/x/net/proxy"
)

// fakeDeviceContainer é o fake mínimo de deviceContainer.
type fakeDeviceContainer struct {
	GetDeviceFn func(ctx context.Context, jid types.JID) (*store.Device, error)
	NewDeviceFn func() *store.Device
}

func (f *fakeDeviceContainer) GetDevice(ctx context.Context, jid types.JID) (*store.Device, error) {
	if f.GetDeviceFn != nil {
		return f.GetDeviceFn(ctx, jid)
	}
	return nil, nil
}

func (f *fakeDeviceContainer) NewDevice() *store.Device {
	if f.NewDeviceFn != nil {
		return f.NewDeviceFn()
	}
	return &store.Device{}
}

// fakeSessionClient é o fake mínimo de sessionClient.
type fakeSessionClient struct {
	GetQRChannelFn    func(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error)
	ConnectFn         func() error
	DisconnectFn      func()
	IsConnectedFn     func() bool
	IsLoggedInFn      func() bool
	LogoutFn          func(ctx context.Context) error
	AddEventHandlerFn func(handler whatsmeow.EventHandler) uint32
	RemoveHandlerFn   func(id uint32) bool
	SetSOCKSProxyFn   func(px proxy.Dialer, opts ...whatsmeow.SetProxyOptions)
	SetProxyAddressFn func(addr string, opts ...whatsmeow.SetProxyOptions) error
}

func (f *fakeSessionClient) GetQRChannel(ctx context.Context) (<-chan whatsmeow.QRChannelItem, error) {
	return f.GetQRChannelFn(ctx)
}
func (f *fakeSessionClient) Connect() error { return f.ConnectFn() }
func (f *fakeSessionClient) Disconnect() {
	if f.DisconnectFn != nil {
		f.DisconnectFn()
	}
}
func (f *fakeSessionClient) IsConnected() bool {
	if f.IsConnectedFn != nil {
		return f.IsConnectedFn()
	}
	return false
}
func (f *fakeSessionClient) IsLoggedIn() bool {
	if f.IsLoggedInFn != nil {
		return f.IsLoggedInFn()
	}
	return false
}
func (f *fakeSessionClient) Logout(ctx context.Context) error {
	if f.LogoutFn != nil {
		return f.LogoutFn(ctx)
	}
	return nil
}
func (f *fakeSessionClient) AddEventHandler(handler whatsmeow.EventHandler) uint32 {
	if f.AddEventHandlerFn != nil {
		return f.AddEventHandlerFn(handler)
	}
	return 0
}
func (f *fakeSessionClient) RemoveEventHandler(id uint32) bool {
	if f.RemoveHandlerFn != nil {
		return f.RemoveHandlerFn(id)
	}
	return true
}
func (f *fakeSessionClient) SetSOCKSProxy(px proxy.Dialer, opts ...whatsmeow.SetProxyOptions) {
	if f.SetSOCKSProxyFn != nil {
		f.SetSOCKSProxyFn(px, opts...)
	}
}
func (f *fakeSessionClient) SetProxyAddress(addr string, opts ...whatsmeow.SetProxyOptions) error {
	if f.SetProxyAddressFn != nil {
		return f.SetProxyAddressFn(addr, opts...)
	}
	return nil
}

func TestSessionProviderAdapter_NewSession_NewDeviceWhenNoJID(t *testing.T) {
	newDeviceCalled := false
	container := &fakeDeviceContainer{
		NewDeviceFn: func() *store.Device {
			newDeviceCalled = true
			return &store.Device{}
		},
	}
	p := NewSessionProviderAdapter(container, nil, func(d *store.Device) sessionClient {
		return &fakeSessionClient{}
	})

	sess, err := p.NewSession(context.Background(), appport.SessionSpec{UserID: "u1"})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if !newDeviceCalled {
		t.Error("expected NewDevice to be called when lookupJID is nil")
	}
	if sess.HasCredentials() {
		t.Error("fresh device should have no credentials")
	}
}

func TestSessionProviderAdapter_NewSession_ReusesDeviceForKnownJID(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	existing := &store.Device{ID: &jid}
	container := &fakeDeviceContainer{
		GetDeviceFn: func(ctx context.Context, gotJID types.JID) (*store.Device, error) {
			if gotJID.String() != jid.String() {
				t.Errorf("GetDevice got %s, want %s", gotJID, jid)
			}
			return existing, nil
		},
	}
	p := NewSessionProviderAdapter(container, func(ctx context.Context, userID string) (string, error) {
		return jid.String(), nil
	}, func(d *store.Device) sessionClient {
		return &fakeSessionClient{}
	})

	sess, err := p.NewSession(context.Background(), appport.SessionSpec{UserID: "u1"})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	got, ok := sess.JID()
	if !ok || got != jid.String() {
		t.Errorf("JID() = %q, %v; want %q, true", got, ok, jid.String())
	}
}

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

func TestWhatsmeowSession_Connect_RequiresCredentials(t *testing.T) {
	s := &whatsmeowSession{device: &store.Device{}, client: &fakeSessionClient{}}
	if err := s.Connect(context.Background()); err == nil {
		t.Error("expected error connecting without credentials")
	}
}

func TestWhatsmeowSession_Connect_Success(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	called := false
	client := &fakeSessionClient{ConnectFn: func() error { called = true; return nil }}
	s := &whatsmeowSession{device: &store.Device{ID: &jid}, client: client}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !called {
		t.Error("expected underlying Connect to be called")
	}
}

func TestWhatsmeowSession_DisconnectIsConnectedIsLoggedIn(t *testing.T) {
	disconnected := false
	client := &fakeSessionClient{
		DisconnectFn:  func() { disconnected = true },
		IsConnectedFn: func() bool { return true },
		IsLoggedInFn:  func() bool { return true },
	}
	s := &whatsmeowSession{device: &store.Device{}, client: client}
	s.Disconnect()
	if !disconnected {
		t.Error("expected Disconnect to be called")
	}
	if !s.IsConnected() {
		t.Error("expected IsConnected true")
	}
	if !s.IsLoggedIn() {
		t.Error("expected IsLoggedIn true")
	}
}

func TestWhatsmeowSession_Logout(t *testing.T) {
	client := &fakeSessionClient{LogoutFn: func(ctx context.Context) error { return nil }}
	s := &whatsmeowSession{device: &store.Device{}, client: client}
	if err := s.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	failing := &fakeSessionClient{LogoutFn: func(ctx context.Context) error { return errors.New("boom") }}
	s2 := &whatsmeowSession{device: &store.Device{}, client: failing}
	if err := s2.Logout(context.Background()); err == nil {
		t.Error("expected error from failing Logout")
	}
}

func TestWhatsmeowSession_SetProxy(t *testing.T) {
	t.Run("rejects after connect", func(t *testing.T) {
		client := &fakeSessionClient{IsConnectedFn: func() bool { return true }}
		s := &whatsmeowSession{device: &store.Device{}, client: client}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeHTTP, URL: "http://proxy:8080"}); err == nil {
			t.Error("expected error setting proxy after connect")
		}
	})

	t.Run("invalid url", func(t *testing.T) {
		s := &whatsmeowSession{device: &store.Device{}, client: &fakeSessionClient{}}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeHTTP, URL: "://bad"}); err == nil {
			t.Error("expected error for invalid proxy URL")
		}
	})

	t.Run("http mode", func(t *testing.T) {
		var gotAddr string
		client := &fakeSessionClient{SetProxyAddressFn: func(addr string, opts ...whatsmeow.SetProxyOptions) error {
			gotAddr = addr
			return nil
		}}
		s := &whatsmeowSession{device: &store.Device{}, client: client}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeHTTP, URL: "http://proxy:8080"}); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		if gotAddr != "http://proxy:8080" {
			t.Errorf("gotAddr = %q", gotAddr)
		}
	})

	t.Run("socks5 mode", func(t *testing.T) {
		called := false
		client := &fakeSessionClient{SetSOCKSProxyFn: func(px proxy.Dialer, opts ...whatsmeow.SetProxyOptions) {
			called = true
		}}
		s := &whatsmeowSession{device: &store.Device{}, client: client}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeSOCKS5, URL: "socks5://127.0.0.1:1080"}); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		if !called {
			t.Error("expected SetSOCKSProxy to be called")
		}
	})

	t.Run("unknown mode", func(t *testing.T) {
		s := &whatsmeowSession{device: &store.Device{}, client: &fakeSessionClient{}}
		if err := s.SetProxy(appport.ProxyConfig{Mode: "bogus", URL: "http://x"}); err == nil {
			t.Error("expected error for unknown proxy mode")
		}
	})
}

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

var (
	_ appport.SessionProvider = (*SessionProviderAdapter)(nil)
	_ appport.Session         = (*whatsmeowSession)(nil)
)
