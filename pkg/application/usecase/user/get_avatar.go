package user

import (
	"context"
	"errors"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetAvatarUseCase retrieves avatar info for a user
type GetAvatarUseCase struct {
	contacts appport.ContactDirectory
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewGetAvatarUseCase creates a new instance
func NewGetAvatarUseCase(cd appport.ContactDirectory, jr appport.JIDResolver, logger appport.Logger) *GetAvatarUseCase {
	return &GetAvatarUseCase{contacts: cd, jids: jr, logger: logger}
}

// Execute retrieves avatar info
func (uc *GetAvatarUseCase) Execute(ctx context.Context, userID string, req domain.GetAvatarRequest) (map[string]interface{}, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		uc.logger.Error(ctx, "no whatsmeow session", "error", err, "user_id", userID)
		return nil, err
	}

	if len(req.Phone) < 1 {
		return nil, fmt.Errorf("missing Phone in Payload")
	}

	jid, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		return nil, fmt.Errorf("could not parse Phone")
	}

	pic, err := uc.contacts.GetProfilePicture(ctx, userID, jid, req.Preview)
	if errors.Is(err, domain.ErrAvatarNotFound) || errors.Is(err, domain.ErrAvatarUnauthorized) {
		// Não é falha — propaga o sentinel intacto (sem fmt.Errorf, que
		// quebraria errors.Is no handler) para o caller distinguir
		// "sem foto" (404) de "escondida por privacidade" (403).
		return nil, err
	}
	if err != nil {
		uc.logger.Error(ctx, "Failed to get avatar", "error", err, "user_id", userID, "phone", req.Phone)
		return nil, fmt.Errorf("failed to get avatar: %v", err)
	}

	if pic == nil {
		return nil, ErrAvatarNotFound
	}

	uc.logger.Info(ctx, "Got avatar", "id", pic.ID, "url", pic.URL, "user_id", userID)

	return map[string]interface{}{
		"id":  pic.ID,
		"url": pic.URL,
	}, nil
}
