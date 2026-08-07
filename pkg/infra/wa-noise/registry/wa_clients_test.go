package registry

import (
	"testing"
	wanoise "wa-api/internal/wa-noise"
)

// TestClientManager_wa-noiseLifecycle: Set → Get → Delete.
func TestClientManager_WaNoiseLifecycle(t *testing.T) {
	cm := NewClientManager()
	wac := &wanoise.Client{}
	cm.SetWaNoiseClient("u1", wac)
	if cm.GetWaNoiseClientsCount() != 1 {
		t.Errorf("count after set = %d, want 1", cm.GetWaNoiseClientsCount())
	}
	if got := cm.GetWaNoiseClient("u1"); got != wac {
		t.Error("GetWaNoiseClient returned different client")
	}
	cm.DeleteWaNoiseClient("u1")
	if cm.GetWaNoiseClientsCount() != 0 {
		t.Errorf("count after delete = %d, want 0", cm.GetWaNoiseClientsCount())
	}
	if got := cm.GetWaNoiseClient("u1"); got != nil {
		t.Error("GetWaNoiseClient after delete returned non-nil")
	}
}

// TestClientManager_GetAllClients devolve snapshot.
func TestClientManager_GetAllClients(t *testing.T) {
	cm := NewClientManager()
	wac1 := &wanoise.Client{}
	wac2 := &wanoise.Client{}
	cm.SetWaNoiseClient("u1", wac1)
	cm.SetWaNoiseClient("u2", wac2)
	all := cm.GetAllClients()
	if len(all) != 2 {
		t.Errorf("GetAllClients = %d, want 2", len(all))
	}
	if all["u1"] != wac1 {
		t.Error("GetAllClients[u1] wrong")
	}
}

// TestClientManager_Iteratewa-noiseClients itera com callback.
func TestClientManager_IterateWaNoiseClients(t *testing.T) {
	cm := NewClientManager()
	cm.SetWaNoiseClient("u1", &wanoise.Client{})
	cm.SetWaNoiseClient("u2", &wanoise.Client{})
	count := 0
	cm.IterateWaNoiseClients(func(c *wanoise.Client) bool {
		count++
		return true
	})
	if count != 2 {
		t.Errorf("IterateWaNoiseClients count = %d, want 2", count)
	}
}

// TestClientManager_Iteratewa-noiseClients_BreakEarly callback false para.
func TestClientManager_IterateWaNoiseClients_BreakEarly(t *testing.T) {
	cm := NewClientManager()
	cm.SetWaNoiseClient("u1", &wanoise.Client{})
	cm.SetWaNoiseClient("u2", &wanoise.Client{})
	cm.SetWaNoiseClient("u3", &wanoise.Client{})
	count := 0
	cm.IterateWaNoiseClients(func(c *wanoise.Client) bool {
		count++
		return count < 2
	})
	if count != 2 {
		t.Errorf("IterateWaNoiseClients count = %d, want 2", count)
	}
}

// TestClientManager_Iteratewa-noiseClients_Empty.
func TestClientManager_IterateWaNoiseClients_Empty(t *testing.T) {
	cm := NewClientManager()
	count := 0
	cm.IterateWaNoiseClients(func(c *wanoise.Client) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("IterateWaNoiseClients(empty) count = %d, want 0", count)
	}
}
