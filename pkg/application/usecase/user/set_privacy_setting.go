package user

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow/types"
)

// SetPrivacySettingUseCase sets a privacy setting
type SetPrivacySettingUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewSetPrivacySettingUseCase creates a new instance
func NewSetPrivacySettingUseCase(cp appport.ClientProvider, logger appport.Logger) *SetPrivacySettingUseCase {
	return &SetPrivacySettingUseCase{clientProvider: cp, logger: logger}
}

// Execute sets a privacy setting with validation
func (uc *SetPrivacySettingUseCase) Execute(ctx context.Context, userID string, req domain.SetPrivacySettingRequest) (interface{}, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	if err := ValidatePrivacySetting(req.PrivacySetting, req.Value); err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	settings, err := client.SetPrivacySetting(ctxWithTimeout, types.PrivacySettingType(req.PrivacySetting), types.PrivacySetting(req.Value))
	if err != nil {
		uc.logger.Error(ctx, "failed to set privacy setting", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to set privacy setting: %w", err)
	}

	return settings, nil
}

var privacySettingValues = map[types.PrivacySettingType][]types.PrivacySetting{
	types.PrivacySettingTypeGroupAdd:     {types.PrivacySettingAll, types.PrivacySettingContacts, types.PrivacySettingContactBlacklist, types.PrivacySettingNone},
	types.PrivacySettingTypeLastSeen:     {types.PrivacySettingAll, types.PrivacySettingContacts, types.PrivacySettingContactBlacklist, types.PrivacySettingNone},
	types.PrivacySettingTypeStatus:       {types.PrivacySettingAll, types.PrivacySettingContacts, types.PrivacySettingContactBlacklist, types.PrivacySettingNone},
	types.PrivacySettingTypeProfile:      {types.PrivacySettingAll, types.PrivacySettingContacts, types.PrivacySettingContactBlacklist, types.PrivacySettingNone},
	types.PrivacySettingTypeReadReceipts: {types.PrivacySettingAll, types.PrivacySettingNone},
	types.PrivacySettingTypeOnline:       {types.PrivacySettingAll, types.PrivacySettingMatchLastSeen},
	types.PrivacySettingTypeCallAdd:      {types.PrivacySettingAll, types.PrivacySettingKnown},
}

// ValidatePrivacySetting reports whether name is a supported privacy setting and
// value is one of the values WhatsApp accepts for it. Exported for testing.
func ValidatePrivacySetting(name, value string) error {
	allowed, ok := privacySettingValues[types.PrivacySettingType(name)]
	if !ok {
		return fmt.Errorf("invalid privacy setting name %q", name)
	}
	for _, v := range allowed {
		if types.PrivacySetting(value) == v {
			return nil
		}
	}
	return fmt.Errorf("invalid value %q for privacy setting %q", value, name)
}
