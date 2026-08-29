package user

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// GetBlocklistUseCase retrieves the current blocklist
type GetBlocklistUseCase struct {
	blocklist appport.BlocklistManager
	logger    appport.Logger
}

// NewGetBlocklistUseCase creates a new instance
func NewGetBlocklistUseCase(bm appport.BlocklistManager, logger appport.Logger) *GetBlocklistUseCase {
	return &GetBlocklistUseCase{blocklist: bm, logger: logger}
}

// Execute retrieves the blocklist
func (uc *GetBlocklistUseCase) Execute(ctx context.Context, userID string, _ domain.GetBlocklistRequest) (domain.Blocklist, error) {
	if err := uc.blocklist.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "error", err, "user_id", userID)
		return domain.Blocklist{}, err
	}

	blocklist, err := uc.blocklist.GetBlocklist(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "Failed to get blocklist", "error", err, "user_id", userID)
		return domain.Blocklist{}, fmt.Errorf("failed to get blocklist: %w", err)
	}

	uc.logger.Info(ctx, "Retrieved blocklist", "user_id", userID, "count", len(blocklist.JIDs))

	// The domain value goes back untouched. The map that used to be built
	// here carried the response's KEY NAMES ("Blocklist", "DHash") inside a
	// use case, which is where a rename broke clients in silence.
	return blocklist, nil
}
