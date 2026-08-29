package registry

import (
	"testing"
	"wa-api/internal/noise"
	clientpkg "wa-api/pkg/infra/noise/client"
)

// fakeUserClient é um UserClient mínimo para os testes do ClientManager.
type fakeUserClient struct{}

func (f *fakeUserClient) GetWAClient() *noise.Client { return nil }
func (f *fakeUserClient) GetUserID() string          { return "fake-user" }

func TestNewClientManager(t *testing.T) {
	cm := NewClientManager()
	if cm == nil {
		t.Fatal("NewClientManager returned nil")
	}
	if cm.GetNoiseClientsCount() != 0 {
		t.Errorf("New manager count = %d, want 0", cm.GetNoiseClientsCount())
	}
}

// Verifica que clientpkg.ClientForGetter devolve getter que mapeia para clientpkg.RealClient.
func TestClientForGetter_NilLookup(t *testing.T) {
	getter := clientpkg.ClientForGetter(func(uid string) *noise.Client { return nil })
	if c := getter("u1"); c != nil {
		t.Error("clientpkg.ClientForGetter(nil lookup) should return nil")
	}
}

func TestClientForGetter_ConcreteClient(t *testing.T) {
	wac := &noise.Client{}
	getter := clientpkg.ClientForGetter(func(uid string) *noise.Client { return wac })
	got := getter("u1")
	if got == nil {
		t.Fatal("clientpkg.ClientForGetter(concrete) returned nil")
	}
	_, ok := got.(clientpkg.RealClient)
	if !ok {
		t.Errorf("clientpkg.ClientForGetter did not return clientpkg.RealClient, got %T", got)
	}
}
