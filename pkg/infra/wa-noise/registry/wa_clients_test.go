package registry

import (
	"testing"
	whatsmeow "wa-api/internal/wa-noise"
)

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
