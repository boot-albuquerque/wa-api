package whatsmeow

import (
	"context"

	"wa-api/internal/waclient/appstate"
	"wa-api/internal/waclient/store"
	"wa-api/internal/waclient/types"
)

func (f *fakeWAClient) RejectCall(ctx context.Context, callFrom types.JID, callID string) error {
	if f.RejectCallFn != nil {
		return f.RejectCallFn(ctx, callFrom, callID)
	}
	return nil
}

func (f *fakeWAClient) SendAppState(ctx context.Context, patch appstate.PatchInfo) error {
	if f.SendAppStateFn != nil {
		return f.SendAppStateFn(ctx, patch)
	}
	return nil
}

func (f *fakeWAClient) FetchAppState(ctx context.Context, name appstate.WAPatchName, fullSync, onlyIfNotSynced bool) error {
	if f.FetchAppStateFn != nil {
		return f.FetchAppStateFn(ctx, name, fullSync, onlyIfNotSynced)
	}
	return nil
}

func (f *fakeWAClient) GetSubscribedNewsletters(ctx context.Context) ([]*types.NewsletterMetadata, error) {
	if f.GetSubscribedNewslettersFn != nil {
		return f.GetSubscribedNewslettersFn(ctx)
	}
	return nil, nil
}

func (f *fakeWAClient) IsConnected() bool {
	if f.IsConnectedFn != nil {
		return f.IsConnectedFn()
	}
	return false
}

func (f *fakeWAClient) IsLoggedIn() bool {
	if f.IsLoggedInFn != nil {
		return f.IsLoggedInFn()
	}
	return false
}

func (f *fakeWAClient) Logout(ctx context.Context) error {
	if f.LogoutFn != nil {
		return f.LogoutFn(ctx)
	}
	return nil
}

func (f *fakeWAClient) Disconnect() {
	if f.DisconnectFn != nil {
		f.DisconnectFn()
	}
}

func (f *fakeWAClient) Store() *store.Device {
	if f.StoreFn != nil {
		return f.StoreFn()
	}
	return nil
}

// getterWith devolve uma waClientGetter que mapeia txtID para o cliente
// correspondente em clients. txtIDs ausentes devolvem nil (que é
