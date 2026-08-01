package notification

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// ListNewsletterUseCase lists subscribed newsletters
type ListNewsletterUseCase struct {
	newsletters appport.NewsletterReader
	logger      appport.Logger
}

// NewListNewsletterUseCase creates a new instance
func NewListNewsletterUseCase(nr appport.NewsletterReader, logger appport.Logger) *ListNewsletterUseCase {
	return &ListNewsletterUseCase{newsletters: nr, logger: logger}
}

// Execute lists subscribed newsletters
func (uc *ListNewsletterUseCase) Execute(ctx context.Context, userID string) (*domain.NewsletterCollection, error) {
	if err := uc.newsletters.EnsureSession(ctx, userID); err != nil {
		return nil, fmt.Errorf("no session")
	}

	newsletter, err := uc.newsletters.ListSubscribed(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "failed to get newsletter list", "error", err, "user_id", userID)
		return nil, fmt.Errorf("failed to get newsletter list: %w", err)
	}

	collection := &domain.NewsletterCollection{
		Newsletter: newsletter,
	}

	return collection, nil
}
