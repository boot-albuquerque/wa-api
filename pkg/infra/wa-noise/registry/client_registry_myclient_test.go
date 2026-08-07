package registry

import "testing"

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
