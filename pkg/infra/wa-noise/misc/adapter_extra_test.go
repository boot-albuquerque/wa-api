package misc

import (
	"context"
	"errors"
	"testing"
	"wa-api/pkg/infra/wa-noise/waclient"
	"wa-api/pkg/infra/wa-noise/waclient/waclienttest"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
)

// TestMiscAdapter_ProfileAccess_OK devolve ProfileDataAccess.
func TestMiscAdapter_ProfileAccess_OK(t *testing.T) {
	fake := &waclienttest.Fake{StoreFn: func() *store.Device { return &store.Device{} }}
	a := NewMiscAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake}))
	pa, err := a.ProfileAccess(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ProfileAccess = %v", err)
	}
	if pa == nil {
		t.Fatal("ProfileAccess returned nil")
	}
}

// TestMiscAdapter_RequestUnavailableMessage_PropagatesError.
func TestMiscAdapter_RequestUnavailableMessage_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &waclienttest.Fake{
		BuildUnavailableMessageFn: func(chat, sender types.JID, id string) *waE2E.Message { return nil },
		SendMessageFn: func(ctx context.Context, to types.JID, msg *waE2E.Message, extra ...wanoise.SendRequestExtra) (wanoise.SendResponse, error) {
			return wanoise.SendResponse{}, sdkErr
		},
	}
	a := NewMiscAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake}))
	_, err := a.RequestUnavailableMessage(context.Background(), "u1", "x@y.com", "z@y.com", "m1")
	if err == nil {
		t.Fatal("RequestUnavailableMessage não propagou erro")
	}
}

// TestMiscAdapter_ListSubscribed_OK_WithItems.
func TestMiscAdapter_ListSubscribed_OK_WithItems(t *testing.T) {
	meta1 := &types.NewsletterMetadata{ID: types.NewJID("a", types.DefaultUserServer)}
	meta2 := &types.NewsletterMetadata{ID: types.NewJID("b", types.DefaultUserServer)}
	fake := &waclienttest.Fake{GetSubscribedNewslettersFn: func(ctx context.Context) ([]*types.NewsletterMetadata, error) {
		return []*types.NewsletterMetadata{meta1, meta2}, nil
	}}
	a := NewMiscAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake}))
	got, err := a.ListSubscribed(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListSubscribed = %v", err)
	}
	if len(got.([]types.NewsletterMetadata)) != 2 {
		t.Errorf("ListSubscribed = %+v", got)
	}
}

// TestMiscAdapter_RejectCall_PropagatesError.
func TestMiscAdapter_RejectCall_PropagatesError(t *testing.T) {
	sdkErr := errors.New("boom")
	fake := &waclienttest.Fake{RejectCallFn: func(ctx context.Context, callFrom types.JID, callID string) error { return sdkErr }}
	a := NewMiscAdapter(waclienttest.GetterWith(map[string]waclient.Client{"u1": fake}))
	err := a.RejectCall(context.Background(), "u1", "x@y.com", "c1")
	if err == nil {
		t.Fatal("RejectCall não propagou erro")
	}
}
