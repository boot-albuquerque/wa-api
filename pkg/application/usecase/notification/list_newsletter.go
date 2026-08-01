package notification

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow/types"
)

// ListNewsletterUseCase lists subscribed newsletters
type ListNewsletterUseCase struct {
	clientProvider appport.ClientProvider
	logger         zerolog.Logger
}

// NewListNewsletterUseCase creates a new instance
func NewListNewsletterUseCase(cp appport.ClientProvider, logger zerolog.Logger) *ListNewsletterUseCase {
	return &ListNewsletterUseCase{clientProvider: cp, logger: logger}
}

// Execute lists subscribed newsletters
func (uc *ListNewsletterUseCase) Execute(ctx context.Context, userID string) (*domain.NewsletterCollection, error) {
	client, err := uc.clientProvider.GetWhatsmeowClient(ctx, userID)
	if err != nil || client == nil {
		return nil, fmt.Errorf("no session")
	}

	resp, err := client.GetSubscribedNewsletters(ctx)
	if err != nil {
		uc.logger.Error().Err(err).Str("user_id", userID).Msg("failed to get newsletter list")
		return nil, fmt.Errorf("failed to get newsletter list: %w", err)
	}

	newsletter := make([]types.NewsletterMetadata, 0, len(resp))
	for _, info := range resp {
		if info != nil {
			newsletter = append(newsletter, *info)
		}
	}

	collection := &domain.NewsletterCollection{
		Newsletter: newsletter,
	}

	return collection, nil
}
