package registry

import "testing"

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
