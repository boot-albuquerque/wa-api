package user

import (
	"context"
	"errors"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetAvatarUseCase retrieves avatar info for a user
type GetAvatarUseCase struct {
	contacts appport.AvatarReader
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewGetAvatarUseCase creates a new instance
func NewGetAvatarUseCase(cd appport.AvatarReader, jr appport.JIDResolver, logger appport.Logger) *GetAvatarUseCase {
	return &GetAvatarUseCase{contacts: cd, jids: jr, logger: logger}
}

// Execute retrieves avatar info
func (uc *GetAvatarUseCase) Execute(ctx context.Context, userID string, req domain.GetAvatarRequest) (*domain.AvatarInfo, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	if len(req.Phone) < 1 {
		return nil, apperr.New("missing_phone", apperr.CategoryValidation, "missing Phone in Payload", false, nil)
	}

	jid, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
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

	// The domain value goes back untouched. Building a map here was what put
	// the KEY NAMES of the public response inside a use case, which is the
	// coupling the DTO layer exists to remove.
	return pic, nil
}
