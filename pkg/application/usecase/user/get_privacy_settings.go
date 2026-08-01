package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
)

// GetPrivacySettingsUseCase retrieves privacy settings
type GetPrivacySettingsUseCase struct {
	privacy appport.PrivacyManager
	logger  appport.Logger
}

// NewGetPrivacySettingsUseCase creates a new instance
func NewGetPrivacySettingsUseCase(pm appport.PrivacyManager, logger appport.Logger) *GetPrivacySettingsUseCase {
	return &GetPrivacySettingsUseCase{privacy: pm, logger: logger}
}

// Execute retrieves privacy settings with timeout
func (uc *GetPrivacySettingsUseCase) Execute(ctx context.Context, userID string) (interface{}, error) {
	if err := uc.privacy.EnsureSession(ctx, userID); err != nil {
		return nil, fmt.Errorf("no session")
	}

	settings, err := uc.privacy.GetPrivacySettings(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get privacy settings", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to get privacy settings: %w", err)
	}

	return settings, nil
}
