package chat

import (
	"context"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/internal/wa-noise/protocol/types"
)

// TestMessageComposerAdapter_NewMessageID_NoSession devolve wasession.ErrNoSession.
func TestMessageComposerAdapter_NewMessageID_NoSession(t *testing.T) {
	a := NewMessageComposerAdapter(testkit.GetterWith(nil))
	_, err := a.NewMessageID(context.Background(), "u1")
	if testkit.AppErrCode(err) != "no_session" {
		t.Errorf("NewMessageID code = %q, want no_session", testkit.AppErrCode(err))
	}
}

// TestMessageComposerAdapter_NewMessageID_OK propaga o id do SDK.
func TestMessageComposerAdapter_NewMessageID_OK(t *testing.T) {
	fake := &testkit.Fake{GenerateMessageIDFn: func() types.MessageID {
		return "ABCDEF123456"
	}}
	a := NewMessageComposerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	id, err := a.NewMessageID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("NewMessageID = %v", err)
	}
	if id != "ABCDEF123456" {
		t.Errorf("NewMessageID = %q, want ABCDEF123456", id)
	}
}
