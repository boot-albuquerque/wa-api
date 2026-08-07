package presence

import (
	"context"
	"errors"
	"testing"
	waclient "wa-api/pkg/infra/wa-noise/client"
	"wa-api/pkg/infra/wa-noise/client/testkit"

	"wa-api/pkg/domain"

	"wa-api/internal/wa-noise/protocol/types"
)

// TestPresenceControllerAdapter_PropagatesAllErrors.
func TestPresenceControllerAdapter_PropagatesAllErrors(t *testing.T) {
	sdkErr := errors.New("sdk boom")
	fake := &testkit.Fake{
		SendPresenceFn: func(ctx context.Context, state types.Presence) error { return sdkErr },
		SendChatPresenceFn: func(ctx context.Context, jid types.JID, state types.ChatPresence, media types.ChatPresenceMedia) error {
			return sdkErr
		},
		SubscribePresenceFn: func(ctx context.Context, jid types.JID) error { return sdkErr },
	}
	a := NewPresenceControllerAdapter(testkit.GetterWith(map[string]waclient.Client{"u1": fake}))
	if err := a.SendPresence(context.Background(), "u1", domain.PresenceAvailable); err == nil {
		t.Fatal("SendPresence não propagou erro")
	}
	if err := a.SendChatPresence(context.Background(), "u1", "x@y.com", "c", ""); err == nil {
		t.Fatal("SendChatPresence não propagou erro")
	}
	if err := a.SubscribePresence(context.Background(), "u1", "x@y.com"); err == nil {
		t.Fatal("SubscribePresence não propagou erro")
	}
}
