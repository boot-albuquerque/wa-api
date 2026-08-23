package testkit

import (
	"context"
	"time"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/types"
)

// The eleven newsletter capabilities the facade gained in the parity work of
// 2026-08-20. Each defaults to a benign zero value so an existing test that
// never touches newsletters keeps compiling and keeps passing.

func (f *Fake) CreateNewsletter(ctx context.Context, params wanoise.CreateNewsletterParams) (*types.NewsletterMetadata, error) {
	if f.CreateNewsletterFn != nil {
		return f.CreateNewsletterFn(ctx, params)
	}
	return nil, nil
}

func (f *Fake) GetNewsletterInfo(ctx context.Context, jid types.JID) (*types.NewsletterMetadata, error) {
	if f.GetNewsletterInfoFn != nil {
		return f.GetNewsletterInfoFn(ctx, jid)
	}
	return nil, nil
}

func (f *Fake) GetNewsletterInfoWithInvite(ctx context.Context, key string) (*types.NewsletterMetadata, error) {
	if f.GetNewsletterInfoWithInviteFn != nil {
		return f.GetNewsletterInfoWithInviteFn(ctx, key)
	}
	return nil, nil
}

func (f *Fake) FollowNewsletter(ctx context.Context, jid types.JID) error {
	if f.FollowNewsletterFn != nil {
		return f.FollowNewsletterFn(ctx, jid)
	}
	return nil
}

func (f *Fake) UnfollowNewsletter(ctx context.Context, jid types.JID) error {
	if f.UnfollowNewsletterFn != nil {
		return f.UnfollowNewsletterFn(ctx, jid)
	}
	return nil
}

func (f *Fake) NewsletterToggleMute(ctx context.Context, jid types.JID, mute bool) error {
	if f.NewsletterToggleMuteFn != nil {
		return f.NewsletterToggleMuteFn(ctx, jid, mute)
	}
	return nil
}

func (f *Fake) GetNewsletterMessages(ctx context.Context, jid types.JID, params *wanoise.GetNewsletterMessagesParams) ([]*types.NewsletterMessage, error) {
	if f.GetNewsletterMessagesFn != nil {
		return f.GetNewsletterMessagesFn(ctx, jid, params)
	}
	return nil, nil
}

func (f *Fake) GetNewsletterMessageUpdates(ctx context.Context, jid types.JID, params *wanoise.GetNewsletterUpdatesParams) ([]*types.NewsletterMessage, error) {
	if f.GetNewsletterMessageUpdatesFn != nil {
		return f.GetNewsletterMessageUpdatesFn(ctx, jid, params)
	}
	return nil, nil
}

func (f *Fake) NewsletterMarkViewed(ctx context.Context, jid types.JID, serverIDs []types.MessageServerID) error {
	if f.NewsletterMarkViewedFn != nil {
		return f.NewsletterMarkViewedFn(ctx, jid, serverIDs)
	}
	return nil
}

func (f *Fake) NewsletterSendReaction(ctx context.Context, jid types.JID, serverID types.MessageServerID, reaction string, messageID types.MessageID) error {
	if f.NewsletterSendReactionFn != nil {
		return f.NewsletterSendReactionFn(ctx, jid, serverID, reaction, messageID)
	}
	return nil
}

func (f *Fake) NewsletterSubscribeLiveUpdates(ctx context.Context, jid types.JID) (time.Duration, error) {
	if f.NewsletterSubscribeLiveUpdatesFn != nil {
		return f.NewsletterSubscribeLiveUpdatesFn(ctx, jid)
	}
	return 0, nil
}
