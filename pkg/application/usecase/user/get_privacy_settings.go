package user

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	appport "wa-api/pkg/application/contracts"
)

// GetPrivacySettingsUseCase retrieves privacy settings
type GetPrivacySettingsUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewGetPrivacySettingsUseCase creates a new instance
func NewGetPrivacySettingsUseCase(cp appport.ClientProvider, logger zerolog.Logger) *GetPrivacySettingsUseCase {
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
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to get privacy settings")
		return nil, fmt.Errorf("failed to get privacy settings: %w", err)
	}

	return settings, nil
}
