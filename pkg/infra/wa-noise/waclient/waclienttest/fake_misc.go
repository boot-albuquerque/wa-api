package waclienttest

import (
	"context"

	"wa-api/internal/wa-noise/appstate"
	"wa-api/internal/wa-noise/store"
	"wa-api/internal/wa-noise/types"
)

func (f *Fake) RejectCall(ctx context.Context, callFrom types.JID, callID string) error {
	if f.RejectCallFn != nil {
		return f.RejectCallFn(ctx, callFrom, callID)
	}
	return nil
}

func (f *Fake) SendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	if f.SendAppStateFn != nil {
		return f.SendAppStateFn(ctx, patch)
	}
	return nil
}

func (f *Fake) FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	if f.FetchAppStateFn != nil {
		return f.FetchAppStateFn(ctx, name, fullSync, onlyIfNotSynced)
	}
	return nil
}

func (f *Fake) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	if f.GetSubscribedNewslettersFn != nil {
		return f.GetSubscribedNewslettersFn(ctx)
	}
	return nil, nil
}

func (f *Fake) IsConnected() bool {
	if f.IsConnectedFn != nil {
		return f.IsConnectedFn()
	}
	return false
}

func (f *Fake) IsLoggedIn() bool {
	if f.IsLoggedInFn != nil {
		return f.IsLoggedInFn()
	}
	return false
}

func (f *Fake) Logout(ctx context.Context) error {
	if f.LogoutFn != nil {
		return f.LogoutFn(ctx)
	}
	return nil
}

func (f *Fake) Disconnect() {
	if f.DisconnectFn != nil {
		f.DisconnectFn()
	}
}

func (f *Fake) Store() *store.Device {
	if f.StoreFn != nil {
		return f.StoreFn()
	}
	return nil
}
