package whatsmeow

import (
	"testing"
	whatsmeow "wa-api/internal/waclient"
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

// Verifica que ClientForGetter devolve getter que mapeia para realWAClient.
func TestClientForGetter_NilLookup(t *testing.T) {
	getter := ClientForGetter(func(uid string) *whatsmeow.Client { return nil })
	if c := getter("u1"); c != nil {
		t.Error("ClientForGetter(nil lookup) should return nil")
	}
}

func TestClientForGetter_ConcreteClient(t *testing.T) {
	wac := &whatsmeow.Client{}
	getter := ClientForGetter(func(uid string) *whatsmeow.Client { return wac })
	got := getter("u1")
	if got == nil {
		t.Fatal("ClientForGetter(concrete) returned nil")
	}
	_, ok := got.(realWAClient)
	if !ok {
		t.Errorf("ClientForGetter did not return realWAClient, got %T", got)
	}
}
