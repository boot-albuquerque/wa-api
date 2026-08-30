package registry

import (
	"testing"
	"wa-api/internal/noise"
)

// TestClientManager_noiseLifecycle: Set → Get → Delete.
func TestClientManager_NoiseLifecycle(t *testing.T) {
	cm := NewClientManager()
	wac := &noise.Client{}
	cm.SetNoiseClient("u1", wac)
	if cm.GetNoiseClientsCount() != 1 {
		t.Errorf("count after set = %d, want 1", cm.GetNoiseClientsCount())
	}
	if got := cm.GetNoiseClient("u1"); got != wac {
		t.Error("GetNoiseClient returned different client")
	}
	cm.DeleteNoiseClient("u1")
	if cm.GetNoiseClientsCount() != 0 {
		t.Errorf("count after delete = %d, want 0", cm.GetNoiseClientsCount())
	}
	if got := cm.GetNoiseClient("u1"); got != nil {
		t.Error("GetNoiseClient after delete returned non-nil")
	}
}

// TestClientManager_GetAllClients devolve snapshot.
func TestClientManager_GetAllClients(t *testing.T) {
	cm := NewClientManager()
	wac1 := &noise.Client{}
	wac2 := &noise.Client{}
	cm.SetNoiseClient("u1", wac1)
	cm.SetNoiseClient("u2", wac2)
	all := cm.GetAllClients()
	if len(all) != 2 {
		t.Errorf("GetAllClients = %d, want 2", len(all))
	}
	if all["u1"] != wac1 {
		t.Error("GetAllClients[u1] wrong")
	}
}

// TestClientManager_IterateNoiseClients itera com callback.
func TestClientManager_IterateNoiseClients(t *testing.T) {
	cm := NewClientManager()
	cm.SetNoiseClient("u1", &noise.Client{})
	cm.SetNoiseClient("u2", &noise.Client{})
	count := 0
	cm.IterateNoiseClients(func(c *noise.Client) bool {
		count++
		return true
	})
	if count != 2 {
		t.Errorf("IterateNoiseClients count = %d, want 2", count)
	}
}

// TestClientManager_IterateNoiseClients_BreakEarly callback false para.
func TestClientManager_IterateNoiseClients_BreakEarly(t *testing.T) {
	cm := NewClientManager()
	cm.SetNoiseClient("u1", &noise.Client{})
	cm.SetNoiseClient("u2", &noise.Client{})
	cm.SetNoiseClient("u3", &noise.Client{})
	count := 0
	cm.IterateNoiseClients(func(c *noise.Client) bool {
		count++
		return count < 2
	})
	if count != 2 {
		t.Errorf("IterateNoiseClients count = %d, want 2", count)
	}
}

// TestClientManager_IterateNoiseClients_Empty.
func TestClientManager_IterateNoiseClients_Empty(t *testing.T) {
	cm := NewClientManager()
	count := 0
	cm.IterateNoiseClients(func(c *noise.Client) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("IterateNoiseClients(empty) count = %d, want 0", count)
	}
}
