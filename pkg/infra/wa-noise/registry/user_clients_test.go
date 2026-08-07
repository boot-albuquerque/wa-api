package registry

import "testing"

// TestClientManager_UserClientLifecycle: Set → Get → Delete para UserClient.
func TestClientManager_UserClientLifecycle(t *testing.T) {
	cm := NewClientManager()
	mc := &fakeUserClient{}
	cm.SetUserClient("u1", mc)
	if got := cm.GetUserClient("u1"); got != mc {
		t.Error("GetUserClient returned different client")
	}
	cm.DeleteUserClient("u1")
	if got := cm.GetUserClient("u1"); got != nil {
		t.Error("GetUserClient after delete returned non-nil")
	}
}

// TestClientManager_PollOptionsLifecycle cobre o ramo de poll options
// do DeleteUserClient, que também limpa o cache de opções.
func TestClientManager_PollOptionsLifecycle(t *testing.T) {
	cm := NewClientManager()
	mc := &fakeUserClient{}
	cm.SetUserClient("u1", mc)
	cm.SetPollOptions("u1", "msg-1", []string{"a", "b"})
	if got := cm.GetPollOptions("u1", "msg-1"); len(got) != 2 {
		t.Errorf("GetPollOptions before delete = %v, want 2 items", got)
	}
	cm.DeleteUserClient("u1")
	if got := cm.GetPollOptions("u1", "msg-1"); got != nil {
		t.Errorf("GetPollOptions after delete = %v, want nil", got)
	}
}
