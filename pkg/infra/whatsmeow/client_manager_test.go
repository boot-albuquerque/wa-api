package whatsmeow

import (
	"testing"

	"github.com/go-resty/resty/v2"
	"go.mau.fi/whatsmeow"
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

// TestClientManager_WhatsmeowLifecycle: Set → Get → Delete.
func TestClientManager_WhatsmeowLifecycle(t *testing.T) {
	cm := NewClientManager()
	wac := &whatsmeow.Client{}
	cm.SetWhatsmeowClient("u1", wac)
	if cm.GetWhatsmeowClientsCount() != 1 {
		t.Errorf("count after set = %d, want 1", cm.GetWhatsmeowClientsCount())
	}
	if got := cm.GetWhatsmeowClient("u1"); got != wac {
		t.Error("GetWhatsmeowClient returned different client")
	}
	cm.DeleteWhatsmeowClient("u1")
	if cm.GetWhatsmeowClientsCount() != 0 {
		t.Errorf("count after delete = %d, want 0", cm.GetWhatsmeowClientsCount())
	}
	if got := cm.GetWhatsmeowClient("u1"); got != nil {
		t.Error("GetWhatsmeowClient after delete returned non-nil")
	}
}

// TestClientManager_HTTPLifecycle: Set → Get → Delete para HTTP clients.
func TestClientManager_HTTPLifecycle(t *testing.T) {
	cm := NewClientManager()
	hc := resty.New()
	cm.SetHTTPClient("u1", hc)
	if got := cm.GetHTTPClient("u1"); got != hc {
		t.Error("GetHTTPClient returned different client")
	}
	cm.DeleteHTTPClient("u1")
	if got := cm.GetHTTPClient("u1"); got != nil {
		t.Error("GetHTTPClient after delete returned non-nil")
	}
}

// TestClientManager_MyClientLifecycle: Set → Get → Delete para MyClient.
func TestClientManager_MyClientLifecycle(t *testing.T) {
	cm := NewClientManager()
	mc := &fakeMyClient{}
	cm.SetMyClient("u1", mc)
	if got := cm.GetMyClient("u1"); got != mc {
		t.Error("GetMyClient returned different client")
	}
	cm.DeleteMyClient("u1")
	if got := cm.GetMyClient("u1"); got != nil {
		t.Error("GetMyClient after delete returned non-nil")
	}
}

// TestClientManager_PollOptionsLifecycle cobre o ramo de poll options
// do DeleteMyClient, que também limpa o cache de opções.
func TestClientManager_PollOptionsLifecycle(t *testing.T) {
	cm := NewClientManager()
	mc := &fakeMyClient{}
	cm.SetMyClient("u1", mc)
	cm.SetPollOptions("u1", "msg-1", []string{"a", "b"})
	if got := cm.GetPollOptions("u1", "msg-1"); len(got) != 2 {
		t.Errorf("GetPollOptions before delete = %v, want 2 items", got)
	}
	cm.DeleteMyClient("u1")
	if got := cm.GetPollOptions("u1", "msg-1"); got != nil {
		t.Errorf("GetPollOptions after delete = %v, want nil", got)
	}
}

// TestClientManager_SetPollOptions_InitializaMap cobre a inicialização lazy.
func TestClientManager_SetPollOptions_InitializaMap(t *testing.T) {
	cm := NewClientManager()
	cm.SetPollOptions("u1", "msg-1", []string{"a"})
	// Segunda chamada sem mexer no estado.
	cm.SetPollOptions("u1", "msg-2", []string{"b"})
	if got := cm.GetPollOptions("u1", "msg-2"); len(got) != 1 {
		t.Errorf("GetPollOptions[msg-2] = %v", got)
	}
}

// TestClientManager_GetPollOptions_NoEntry devolve nil.
func TestClientManager_GetPollOptions_NoEntry(t *testing.T) {
	cm := NewClientManager()
	if got := cm.GetPollOptions("u1", "msg-1"); got != nil {
		t.Errorf("GetPollOptions(empty) = %v, want nil", got)
	}
}

// TestClientManager_GetAllClients devolve snapshot.
func TestClientManager_GetAllClients(t *testing.T) {
	cm := NewClientManager()
	wac1 := &whatsmeow.Client{}
	wac2 := &whatsmeow.Client{}
	cm.SetWhatsmeowClient("u1", wac1)
	cm.SetWhatsmeowClient("u2", wac2)
	all := cm.GetAllClients()
	if len(all) != 2 {
		t.Errorf("GetAllClients = %d, want 2", len(all))
	}
	if all["u1"] != wac1 {
		t.Error("GetAllClients[u1] wrong")
	}
}

// TestClientManager_IterateWhatsmeowClients itera com callback.
func TestClientManager_IterateWhatsmeowClients(t *testing.T) {
	cm := NewClientManager()
	cm.SetWhatsmeowClient("u1", &whatsmeow.Client{})
	cm.SetWhatsmeowClient("u2", &whatsmeow.Client{})
	count := 0
	cm.IterateWhatsmeowClients(func(c *whatsmeow.Client) bool {
		count++
		return true
	})
	if count != 2 {
		t.Errorf("IterateWhatsmeowClients count = %d, want 2", count)
	}
}

// TestClientManager_IterateWhatsmeowClients_BreakEarly callback false para.
func TestClientManager_IterateWhatsmeowClients_BreakEarly(t *testing.T) {
	cm := NewClientManager()
	cm.SetWhatsmeowClient("u1", &whatsmeow.Client{})
	cm.SetWhatsmeowClient("u2", &whatsmeow.Client{})
	cm.SetWhatsmeowClient("u3", &whatsmeow.Client{})
	count := 0
	cm.IterateWhatsmeowClients(func(c *whatsmeow.Client) bool {
		count++
		return count < 2
	})
	if count != 2 {
		t.Errorf("IterateWhatsmeowClients count = %d, want 2", count)
	}
}

// TestClientManager_IterateWhatsmeowClients_Empty.
func TestClientManager_IterateWhatsmeowClients_Empty(t *testing.T) {
	cm := NewClientManager()
	count := 0
	cm.IterateWhatsmeowClients(func(c *whatsmeow.Client) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("IterateWhatsmeowClients(empty) count = %d, want 0", count)
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
