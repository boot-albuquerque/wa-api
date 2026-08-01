package user

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
)

// GetPrivacySettingsUseCase retrieves privacy settings
type GetPrivacySettingsUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetPrivacySettingsUseCase creates a new instance
func NewGetPrivacySettingsUseCase(cp appport.ClientProvider, logger appport.Logger) *GetPrivacySettingsUseCase {
	return &GetPrivacySettingsUseCase{clientProvider: cp, logger: logger}
}

// Execute retrieves privacy settings with timeout
func (uc *GetPrivacySettingsUseCase) Execute(ctx context.Context, userID string) (interface{}, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	settings, err := client.TryFetchPrivacySettings(ctxWithTimeout, false)
	if err != nil {
		uc.logger.Error(ctx, "failed to get privacy settings", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to get privacy settings: %w", err)
	}

	return settings, nil
}
