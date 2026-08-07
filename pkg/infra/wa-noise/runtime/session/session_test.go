package session

import (
	"context"
	"errors"
	"testing"

	appport "wa-api/pkg/application/contracts"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/types"

	"golang.org/x/net/proxy"
)

func TestWaNoiseSession_Connect_RequiresCredentials(t *testing.T) {
	s := &wanoiseSession{device: &store.Device{}, client: &fakeSessionClient{}}
	if err := s.Connect(context.Background()); err == nil {
		t.Error("expected error connecting without credentials")
	}
}

func TestWaNoiseSession_Connect_Success(t *testing.T) {
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	called := false
	client := &fakeSessionClient{ConnectFn: func() error { called = true; return nil }}
	s := &wanoiseSession{device: &store.Device{ID: &jid}, client: client}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !called {
		t.Error("expected underlying Connect to be called")
	}
}

func TestWaNoiseSession_DisconnectIsConnectedIsLoggedIn(t *testing.T) {
	disconnected := false
	client := &fakeSessionClient{
		DisconnectFn:  func() { disconnected = true },
		IsConnectedFn: func() bool { return true },
		IsLoggedInFn:  func() bool { return true },
	}
	s := &wanoiseSession{device: &store.Device{}, client: client}
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

func TestWaNoiseSession_Logout(t *testing.T) {
	client := &fakeSessionClient{LogoutFn: func(ctx context.Context) error { return nil }}
	s := &wanoiseSession{device: &store.Device{}, client: client}
	if err := s.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	failing := &fakeSessionClient{LogoutFn: func(ctx context.Context) error { return errors.New("boom") }}
	s2 := &wanoiseSession{device: &store.Device{}, client: failing}
	if err := s2.Logout(context.Background()); err == nil {
		t.Error("expected error from failing Logout")
	}
}

func TestWaNoiseSession_SetProxy(t *testing.T) {
	t.Run("rejects after connect", func(t *testing.T) {
		client := &fakeSessionClient{IsConnectedFn: func() bool { return true }}
		s := &wanoiseSession{device: &store.Device{}, client: client}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeHTTP, URL: "http://proxy:8080"}); err == nil {
			t.Error("expected error setting proxy after connect")
		}
	})

	t.Run("invalid url", func(t *testing.T) {
		s := &wanoiseSession{device: &store.Device{}, client: &fakeSessionClient{}}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeHTTP, URL: "://bad"}); err == nil {
			t.Error("expected error for invalid proxy URL")
		}
	})

	t.Run("http mode", func(t *testing.T) {
		var gotAddr string
		client := &fakeSessionClient{SetProxyAddressFn: func(addr string, opts ...wanoise.SetProxyOptions) error {
			gotAddr = addr
			return nil
		}}
		s := &wanoiseSession{device: &store.Device{}, client: client}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeHTTP, URL: "http://proxy:8080"}); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		if gotAddr != "http://proxy:8080" {
			t.Errorf("gotAddr = %q", gotAddr)
		}
	})

	t.Run("socks5 mode", func(t *testing.T) {
		called := false
		client := &fakeSessionClient{SetSOCKSProxyFn: func(px proxy.Dialer, opts ...wanoise.SetProxyOptions) {
			called = true
		}}
		s := &wanoiseSession{device: &store.Device{}, client: client}
		if err := s.SetProxy(appport.ProxyConfig{Mode: appport.ProxyModeSOCKS5, URL: "socks5://127.0.0.1:1080"}); err != nil {
			t.Fatalf("SetProxy: %v", err)
		}
		if !called {
			t.Error("expected SetSOCKSProxy to be called")
		}
	})

	t.Run("unknown mode", func(t *testing.T) {
		s := &wanoiseSession{device: &store.Device{}, client: &fakeSessionClient{}}
		if err := s.SetProxy(appport.ProxyConfig{Mode: "bogus", URL: "http://x"}); err == nil {
			t.Error("expected error for unknown proxy mode")
		}
	})
}
