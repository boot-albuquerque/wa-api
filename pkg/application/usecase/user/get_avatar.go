package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/whatsmeow"

	wa "go.mau.fi/whatsmeow"
)

// GetAvatarUseCase retrieves avatar info for a user
type GetAvatarUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetAvatarUseCase creates a new instance
func NewGetAvatarUseCase(cp appport.ClientProvider, logger appport.Logger) *GetAvatarUseCase {
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
		uc.logger.Error(ctx, "Failed to get avatar", "error", err, "user_id", userID, "phone", req.Phone)
		return nil, fmt.Errorf("failed to get avatar: %v", err)
	}

	if pic == nil {
		return nil, fmt.Errorf("no avatar found")
	}

	uc.logger.Info(ctx, "Got avatar", "id", pic.ID, "url", pic.URL, "user_id", userID)

	return map[string]interface{}{
		"id":  pic.ID,
		"url": pic.URL,
	}, nil
}
