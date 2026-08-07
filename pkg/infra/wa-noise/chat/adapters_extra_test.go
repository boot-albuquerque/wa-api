package chat

import (
	"context"
	"errors"
	"testing"
	"time"
	"wa-api/pkg/infra/wa-noise/waclient"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"

	"wa-api/pkg/domain"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

// TestChatAdapter_PropagatesErrors.
func TestChatAdapter_PropagatesErrors(t *testing.T) {
	sdkErr := errors.New("sdk boom")
	fake := &waclienttest.Fake{MarkReadFn: func(ctx context.Context, ids []types.MessageID, ts time.Time, chat, sender types.JID, extra ...types.ReceiptType) error {
		return sdkErr
	}}
	a := NewChatMessengerAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake}))
	if err := a.MarkRead(context.Background(), "u1", []string{"m1"}, time.Now(), "x@y.com", "z@y.com"); err == nil {
		t.Fatal("MarkRead não propagou erro")
	}

	fake2 := &waclienttest.Fake{SendMessageFn: func(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
		return wanoise.SendResponse{}, sdkErr
	}}
	a2 := NewChatMessengerAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake2}))
	if _, err := a2.SendReaction(context.Background(), "u1", "x@y.com", domain.Reaction{Text: "👍"}); err == nil {
		t.Fatal("SendReaction não propagou erro")
	}
}
