package whatsmeow

import (
	"context"
	"testing"
	"wa-api/pkg/infra/wa-noise/waclient"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"

	"wa-api/internal/wa-noise/types"
)

// TestMessageComposerAdapter_NewMessageID_NoSession devolve wasession.ErrNoSession.
func TestMessageComposerAdapter_NewMessageID_NoSession(t *testing.T) {
	a := NewMessageComposerAdapter(waclienttest.GetterWith(nil))
	_, err := a.NewMessageID(context.Background(), "u1")
	if waclienttest.AppErrCode(err) != "no_session" {
		t.Errorf("NewMessageID code = %q, want no_session", waclienttest.AppErrCode(err))
	}
}

// TestMessageComposerAdapter_NewMessageID_OK propaga o id do SDK.
func TestMessageComposerAdapter_NewMessageID_OK(t *testing.T) {
	fake := &waclienttest.Fake{GenerateMessageIDFn: func() types.MessageID {
		return "ABCDEF123456"
	}}
	a := NewMessageComposerAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake}))
	id, err := a.NewMessageID(context.Background(), "u1")
	if err != nil {
		t.Fatalf("NewMessageID = %v", err)
	}
	if id != "ABCDEF123456" {
		t.Errorf("NewMessageID = %q, want ABCDEF123456", id)
	}
}
