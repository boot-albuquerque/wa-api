package testkit

import (
	"context"

	wanoise "wa-api/internal/wa-noise"
	"wa-api/internal/wa-noise/protocol/types"
	"wa-api/internal/wa-noise/protocol/types/events"
)

func (f *Fake) IsOnWhatsApp(ctx context.Context, phones []string) ([]types.IsOnWhatsAppResponse, error) {
	if f.IsOnWhatsAppFn != nil {
		return f.IsOnWhatsAppFn(ctx, phones)
	}
	return nil, nil
}

func (f *Fake) GetUserInfo(ctx context.Context, jids []types.JID) (map[types.JID]types.UserInfo, error) {
	if f.GetUserInfoFn != nil {
		return f.GetUserInfoFn(ctx, jids)
	}
	return nil, nil
}

func (f *Fake) GetProfilePictureInfo(ctx context.Context, jid types.JID, params *wanoise.GetProfilePictureParams) (*types.ProfilePictureInfo, error) {
	if f.GetProfilePictureInfoFn != nil {
		return f.GetProfilePictureInfoFn(ctx, jid, params)
	}
	return nil, nil
}

func (f *Fake) GetBlocklist(ctx context.Context) (*types.Blocklist, error) {
	if f.GetBlocklistFn != nil {
		return f.GetBlocklistFn(ctx)
	}
	return nil, nil
}

func (f *Fake) UpdateBlocklist(ctx context.Context, jid types.JID, pnJID types.JID, action events.BlocklistChangeAction) (*types.Blocklist, error) {
	if f.UpdateBlocklistFn != nil {
		return f.UpdateBlocklistFn(ctx, jid, pnJID, action)
	}
	return nil, nil
}

func (f *Fake) TryFetchPrivacySettings(ctx context.Context, ignoreCache bool) (*types.PrivacySettings, error) {
	if f.TryFetchPrivacySettingsFn != nil {
		return f.TryFetchPrivacySettingsFn(ctx, ignoreCache)
	}
	return nil, nil
}

func (f *Fake) SetPrivacySetting(ctx context.Context, name types.PrivacySettingType, value types.PrivacySetting) (types.PrivacySettings, error) {
	if f.SetPrivacySettingFn != nil {
		return f.SetPrivacySettingFn(ctx, name, value)
	}
	return types.PrivacySettings{}, nil
}
