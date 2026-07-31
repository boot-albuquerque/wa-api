package user

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetContactsUseCase retrieves all contacts
type GetContactsUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewGetContactsUseCase creates a new instance
func NewGetContactsUseCase(cp appport.ClientProvider, logger zerolog.Logger) *GetContactsUseCase {
	return &GetContactsUseCase{clientProvider: cp, logger: logger}
}

// Execute retrieves all contacts
func (uc *GetContactsUseCase) Execute(ctx context.Context, userID string, _ domain.GetContactsRequest) (map[types.JID]types.ContactInfo, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	result := make(map[types.JID]types.ContactInfo)
	result, err = client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("Failed to get contacts")
		return nil, err
	}

	uc.logger.Info().Str("user_id", userID).Int("count", len(result)).Msg("Retrieved contacts")
	return result, nil
}
