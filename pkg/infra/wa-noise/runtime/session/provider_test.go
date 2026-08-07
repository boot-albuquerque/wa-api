package session

import (
	"context"
	"testing"

	appport "wa-api/pkg/application/contracts"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"

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
	GetQRChannelFn    func(ctx context.Context) (<-chan wanoise.QRChannelItem, error)
	ConnectFn         func() error
	DisconnectFn      func()
	IsConnectedFn     func() bool
	IsLoggedInFn      func() bool
	LogoutFn          func(ctx context.Context) error
	AddEventHandlerFn func(handler wanoise.EventHandler) uint32
	RemoveHandlerFn   func(id uint32) bool
	SetSOCKSProxyFn   func(px proxy.Dialer, opts ...wanoise.SetProxyOptions)
	SetProxyAddressFn func(addr string, opts ...wanoise.SetProxyOptions) error
}

func (f *fakeSessionClient) GetQRChannel(ctx context.Context) (<-chan wanoise.QRChannelItem, error) {
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
func (f *fakeSessionClient) AddEventHandler(handler wanoise.EventHandler) uint32 {
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
func (f *fakeSessionClient) SetSOCKSProxy(px proxy.Dialer, opts ...wanoise.SetProxyOptions) {
	if f.SetSOCKSProxyFn != nil {
		f.SetSOCKSProxyFn(px, opts...)
	}
}
func (f *fakeSessionClient) SetProxyAddress(addr string, opts ...wanoise.SetProxyOptions) error {
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

var (
	_ appport.SessionProvider = (*SessionProviderAdapter)(nil)
	_ appport.Session         = (*wanoiseSession)(nil)
)
