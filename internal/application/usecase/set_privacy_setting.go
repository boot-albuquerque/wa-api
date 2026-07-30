package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
	appport "wuzapi/internal/application/port"
	"wuzapi/internal/domain"
)

// SetPrivacySettingUseCase sets a privacy setting
type SetPrivacySettingUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewSetPrivacySettingUseCase creates a new instance
func NewSetPrivacySettingUseCase(cp appport.ClientProvider, logger zerolog.Logger) *SetPrivacySettingUseCase {
	return &SetPrivacySettingUseCase{clientProvider: cp, logger: logger}
}

// Execute sets a privacy setting with validation
func (uc *SetPrivacySettingUseCase) Execute(ctx context.Context, userID string, req domain.SetPrivacySettingRequest) (interface{}, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	if err := validatePrivacySetting(req.PrivacySetting, req.Value); err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	settings, err := client.SetPrivacySetting(ctxWithTimeout, types.PrivacySettingType(req.PrivacySetting), types.PrivacySetting(req.Value))
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to set privacy setting")
		return nil, fmt.Errorf("failed to set privacy setting: %w", err)
	}

	return settings, nil
}

// validatePrivacySetting validates privacy setting name and value
func validatePrivacySetting(name, value string) error {
	// Delegate to the existing validation function from handlers.go
	// This will be implemented in handlers.go and imported
	// For now, we'll keep it minimal and let it be called from there
	if name == "" {
		return fmt.Errorf("privacy setting name cannot be empty")
	}
	if value == "" {
		return fmt.Errorf("privacy setting value cannot be empty")
	}
	return nil
}
