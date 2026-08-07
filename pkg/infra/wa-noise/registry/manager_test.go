package registry

import (
	"testing"
	whatsmeow "wa-api/internal/wa-noise/core"
	"wa-api/pkg/infra/wa-noise/waclient"
)

// fakeMyClient é um MyClient mínimo para os testes do ClientManager.
type fakeMyClient struct{}

func (f *fakeMyClient) GetWAClient() *whatsmeow.Client { return nil }
func (f *fakeMyClient) GetUserID() string              { return "fake-user" }

func TestNewClientManager(t *testing.T) {
	cm := NewClientManager()
	if cm == nil {
		t.Fatal("NewClientManager returned nil")
	}
	if cm.GetWhatsmeowClientsCount() != 0 {
		t.Errorf("New manager count = %d, want 0", cm.GetWhatsmeowClientsCount())
	}
}

// Verifica que waclient.ClientForGetter devolve getter que mapeia para waclient.RealClient.
func TestClientForGetter_NilLookup(t *testing.T) {
	getter := waclient.ClientForGetter(func(uid string) *whatsmeow.Client { return nil })
	if c := getter("u1"); c != nil {
		t.Error("waclient.ClientForGetter(nil lookup) should return nil")
	}
}

func TestClientForGetter_ConcreteClient(t *testing.T) {
	wac := &whatsmeow.Client{}
	getter := waclient.ClientForGetter(func(uid string) *whatsmeow.Client { return wac })
	got := getter("u1")
	if got == nil {
		t.Fatal("waclient.ClientForGetter(concrete) returned nil")
	}
	_, ok := got.(waclient.RealClient)
	if !ok {
		t.Errorf("waclient.ClientForGetter did not return waclient.RealClient, got %T", got)
	}
}
