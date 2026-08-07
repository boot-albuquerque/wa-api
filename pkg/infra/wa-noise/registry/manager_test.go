package registry

import (
	"testing"
	wanoise "wa-api/internal/wa-noise"
	waclient "wa-api/pkg/infra/wa-noise/client"
)

// fakeUserClient é um UserClient mínimo para os testes do ClientManager.
type fakeUserClient struct{}

func (f *fakeUserClient) GetWAClient() *wanoise.Client { return nil }
func (f *fakeUserClient) GetUserID() string            { return "fake-user" }

func TestNewClientManager(t *testing.T) {
	cm := NewClientManager()
	if cm == nil {
		t.Fatal("NewClientManager returned nil")
	}
	if cm.GetWaNoiseClientsCount() != 0 {
		t.Errorf("New manager count = %d, want 0", cm.GetWaNoiseClientsCount())
	}
}

// Verifica que waclient.ClientForGetter devolve getter que mapeia para waclient.RealClient.
func TestClientForGetter_NilLookup(t *testing.T) {
	getter := waclient.ClientForGetter(func(uid string) *wanoise.Client { return nil })
	if c := getter("u1"); c != nil {
		t.Error("waclient.ClientForGetter(nil lookup) should return nil")
	}
}

func TestClientForGetter_ConcreteClient(t *testing.T) {
	wac := &wanoise.Client{}
	getter := waclient.ClientForGetter(func(uid string) *wanoise.Client { return wac })
	got := getter("u1")
	if got == nil {
		t.Fatal("waclient.ClientForGetter(concrete) returned nil")
	}
	_, ok := got.(waclient.RealClient)
	if !ok {
		t.Errorf("waclient.ClientForGetter did not return waclient.RealClient, got %T", got)
	}
}
