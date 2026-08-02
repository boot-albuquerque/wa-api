package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// SetPrivacySettingUseCase sets a privacy setting
type SetPrivacySettingUseCase struct {
	privacy appport.PrivacyManager
	logger  appport.Logger
}

// NewSetPrivacySettingUseCase creates a new instance
func NewSetPrivacySettingUseCase(pm appport.PrivacyManager, logger appport.Logger) *SetPrivacySettingUseCase {
	return &SetPrivacySettingUseCase{privacy: pm, logger: logger}
}

// Execute sets a privacy setting with validation
func (uc *SetPrivacySettingUseCase) Execute(ctx context.Context, userID string, req domain.SetPrivacySettingRequest) (interface{}, error) {
	if err := uc.privacy.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	if err := domain.ValidatePrivacySetting(req.PrivacySetting, req.Value); err != nil {
		return nil, err
	}

	settings, err := uc.privacy.SetPrivacySetting(ctx, userID, req.PrivacySetting, req.Value)
	if err != nil {
		uc.logger.Error(ctx, "failed to set privacy setting", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to set privacy setting: %w", err)
	}

	return settings, nil
}
