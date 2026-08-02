package whatsmeow

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// TestMessageComposerAdapter_NewMessageID_NoSession devolve ErrNoSession.
func TestMessageComposerAdapter_NewMessageID_NoSession(t *testing.T) {
	a := NewMessageComposerAdapter(getterWith(nil))
	_, err := a.NewMessageID(context.Background(), "u1")
	if appErrCode(err) != "no_session" {
		t.Errorf("NewMessageID code = %q, want no_session", appErrCode(err))
	}
}

// TestMessageComposerAdapter_NewMessageID_OK propaga o id do SDK.
func TestMessageComposerAdapter_NewMessageID_OK(t *testing.T) {
	fake := &fakeWAClient{GenerateMessageIDFn: func() types.MessageID {
		return "ABCDEF123456"
	}}
	a := NewMessageComposerAdapter(getterWith(map[string]waClient{"u1": fake}))
	id, err := a.NewMessageID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("NewMessageID = %v", err)
	}
	if id != "ABCDEF123456" {
		t.Errorf("NewMessageID = %q, want ABCDEF123456", id)
	}
}