package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetContactsUseCase retrieves all contacts
type GetContactsUseCase struct {
	contacts appport.ContactDirectory
	logger   appport.Logger
}

// NewGetContactsUseCase creates a new instance
func NewGetContactsUseCase(cd appport.ContactDirectory, logger appport.Logger) *GetContactsUseCase {
	return &GetContactsUseCase{contacts: cd, logger: logger}
}

// Execute retrieves all contacts
func (uc *GetContactsUseCase) Execute(ctx context.Context, userID string, _ domain.GetContactsRequest) (interface{}, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		return nil, fmt.Errorf("no session")
	}

	result, count, err := uc.contacts.GetAllContacts(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get contacts", "error", err, "user_id", userID)
		return nil, err
	}

	uc.logger.Info(ctx, "Retrieved contacts", "user_id", userID, "count", count)
	return result, nil
}
