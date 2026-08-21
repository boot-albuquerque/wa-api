package testkit

import (
	"context"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/persistence/store"
	"wa-api/internal/wa-noise/protocol/appstate"
	waE2E "wa-api/internal/wa-noise/protocol/proto/waE2E"
	"wa-api/internal/wa-noise/protocol/types"
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

func (f *Fake) BuildHistorySyncRequest(info *types.MessageInfo, count int) *waE2E.Message {
	if f.BuildHistorySyncRequestFn != nil {
		return f.BuildHistorySyncRequestFn(info, count)
	}
	return &waE2E.Message{}
}

func (f *Fake) SendPeerMessage(ctx context.Context, message *waE2E.Message) (wanoise.SendResponse, error) {
	if f.SendPeerMessageFn != nil {
		return f.SendPeerMessageFn(ctx, message)
	}
	return wanoise.SendResponse{}, nil
}

func (f *Fake) SetStatusMessage(ctx context.Context, msg string) error {
	if f.SetStatusMessageFn != nil {
		return f.SetStatusMessageFn(ctx, msg)
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
