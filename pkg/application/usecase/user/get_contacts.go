package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"go.mau.fi/whatsmeow/types"
)

// GetContactsUseCase retrieves all contacts
type GetContactsUseCase struct {
	clientProvider appport.ClientProvider
	logger         appport.Logger
}

// NewGetContactsUseCase creates a new instance
func NewGetContactsUseCase(cp appport.ClientProvider, logger appport.Logger) *GetContactsUseCase {
	return &GetContactsUseCase{clientProvider: cp, logger: logger}
}

// Execute retrieves all contacts
func (uc *GetContactsUseCase) Execute(ctx context.Context, userID string, _ domain.GetContactsRequest) (map[types.JID]types.ContactInfo, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	result, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get contacts", "error", err, "user_id", userID)
		return nil, err
	}

	uc.logger.Info(ctx, "Retrieved contacts", "user_id", userID, "count", len(result))
	return result, nil
}
