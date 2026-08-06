package whatsmeow

import (
	"context"

	whatsmeow "wa-api/internal/waclient"
	"wa-api/internal/waclient/types"
	"wa-api/internal/waclient/types/events"
)

func (f *fakeWAClient) IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	if f.IsOnWhatsAppFn != nil {
		return f.IsOnWhatsAppFn(ctx, phones)
	}
	return nil, nil
}

func (f *fakeWAClient) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	if f.GetUserInfoFn != nil {
		return f.GetUserInfoFn(ctx, jids)
	}
	return nil, nil
}

func (f *fakeWAClient) GetProfilePictureInfo(ctx context.Context, jid types.JID, params *whatsmeow.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
	if f.GetProfilePictureInfoFn != nil {
		return f.GetProfilePictureInfoFn(ctx, jid, params)
	}
	return nil, nil
}

func (f *fakeWAClient) GetBlocklist(ctx context.Context) (*types.Blocklist, error) {
	if f.GetBlocklistFn != nil {
		return f.GetBlocklistFn(ctx)
	}
	return nil, nil
}

func (f *fakeWAClient) UpdateBlocklist(ctx context.Context, jid types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
	if f.UpdateBlocklistFn != nil {
		return f.UpdateBlocklistFn(ctx, jid, action)
	}
	return nil, nil
}

func (f *fakeWAClient) TryFetchPrivacySettings(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error) {
	if f.TryFetchPrivacySettingsFn != nil {
		return f.TryFetchPrivacySettingsFn(ctx, ignoreCache)
	}
	return nil, nil
}

func (f *fakeWAClient) SetPrivacySetting(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error) {
	if f.SetPrivacySettingFn != nil {
		return f.SetPrivacySettingFn(ctx, name, value)
	}
	return types.PrivacySettings{}, nil
}
