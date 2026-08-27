package user

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetContactsUseCase retrieves all contacts
type GetContactsUseCase struct {
	contacts appport.ContactRoster
	logger   appport.Logger
}

// NewGetContactsUseCase creates a new instance
func NewGetContactsUseCase(cd appport.ContactRoster, logger appport.Logger) *GetContactsUseCase {
	return &GetContactsUseCase{contacts: cd, logger: logger}
}

// Execute retrieves all contacts
func (uc *GetContactsUseCase) Execute(ctx context.Context, userID string, _ domain.GetContactsRequest) ([]domain.Contact, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	result, count, err := uc.contacts.GetAllContacts(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get contacts", "error", err, "user_id", userID)
		return nil, err
	}

	uc.logger.Info(ctx, "Retrieved contacts", "user_id", userID, "count", count)
	return result, nil
}
