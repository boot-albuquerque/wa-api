package domain

// GetAvatarRequest para POST /user/avatar
type GetAvatarRequest struct {
	Phone   string `json:"Phone"`
	Preview bool   `json:"Preview"`
}

// GetContactsRequest para POST /user/contacts (no fields needed)
type GetContactsRequest struct{}

// GetBlocklistRequest para GET /user/blocklist (no fields needed)
type GetBlocklistRequest struct{}

// UpdateBlocklistRequest para POST /user/blocklist/update
type UpdateBlocklistRequest struct {
	Phone string `json:"Phone"`
	JID   string `json:"JID"`
	// Action is inferred from the endpoint (block/unblock)
}

// GetPrivacySettingsRequest para GET /user/privacy (no fields needed)
type GetPrivacySettingsRequest struct{}

// SetPrivacySettingRequest para POST /user/privacy
type SetPrivacySettingRequest struct {
	PrivacySetting string `json:"privacy_setting"`
	Value          string `json:"value"`
}
