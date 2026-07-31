package usecase

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	wa "go.mau.fi/whatsmeow"
	appport "disparazap/internal/application/port"
	"disparazap/internal/domain"
	"disparazap/internal/infrastructure/whatsmeow"
)

// GetAvatarUseCase retrieves avatar info for a user
type GetAvatarUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewGetAvatarUseCase creates a new instance
func NewGetAvatarUseCase(cp appport.ClientProvider, logger zerolog.Logger) *GetAvatarUseCase {
	return &GetAvatarUseCase{clientProvider: cp, logger: logger}
}

// Execute retrieves avatar info
func (uc *GetAvatarUseCase) Execute(ctx context.Context, userID string, req domain.GetAvatarRequest) (map[string]interface{}, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	if len(req.Phone) < 1 {
		return nil, fmt.Errorf("missing Phone in Payload")
	}

	jid, ok := whatsmeow.ParseJID(req.Phone)
	if !ok {
		return nil, fmt.Errorf("could not parse Phone")
	}

	existingID := ""
	pic, err := client.GetProfilePictureInfo(ctx, jid, &wa.GetProfilePictureParams{
		Preview:    req.Preview,
		ExistingID: existingID,
	})

	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Str("phone", req.Phone).Msg("Failed to get avatar")
		return nil, fmt.Errorf("failed to get avatar: %v", err)
	}

	if pic == nil {
		return nil, fmt.Errorf("no avatar found")
	}

	uc.logger.Info().Str("id", pic.ID).Str("url", pic.URL).Str("user_id", userID).Msg("Got avatar")

	return map[string]interface{}{
		"id":  pic.ID,
		"url": pic.URL,
	}, nil
}
